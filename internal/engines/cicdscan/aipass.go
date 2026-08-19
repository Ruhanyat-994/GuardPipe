package cicdscan

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// maxAIWorkflowFiles caps how many workflow files get an AI pass per scan —
// documentation/05-module-specifications.md §10's own budget ("1 call per
// workflow file, capped at 10 files per scan").
const maxAIWorkflowFiles = 10

// lineNearMiss is how close an AI finding's parsed location_hint has to be
// to an already-fired rule finding's line for the AI finding to be
// discarded as a duplicate — §10: "AI findings whose line_hint overlaps a
// rule finding within ±2 lines are discarded."
const lineNearMiss = 2

// digitsInHint pulls the first integer out of an AI response's free-text
// location_hint (the schema only constrains it to a short string, not a
// strict "line N" format) — a best-effort extraction, not a guarantee every
// hint parses; a hint with no parseable line number just skips the
// near-miss dedup for that one finding instead of failing it.
var digitsInHint = regexp.MustCompile(`\d+`)

// runAIPass runs modules/ai's review_workflow prompt against every workflow
// file (up to maxAIWorkflowFiles), told which rule IDs already fired on
// that specific file so the model doesn't repeat them, and discards any
// response finding whose location_hint lands within lineNearMiss lines of
// one that did anyway. aiSvc may be nil (GUARDPIPE_AI_ENABLED=false, or no
// Gemini key configured) — that's the "Gemini unavailable" failure mode
// (§10's own table): rule findings are unaffected, this just doesn't run.
func runAIPass(ctx context.Context, aiSvc ai.Service, in domain.ScanInput, workflows []Workflow, rawContent map[string][]byte, firedLinesByFile map[string][]int, emit func(domain.Finding)) (ran bool, unavailable bool) {
	if aiSvc == nil {
		return false, true
	}

	files := workflows
	if len(files) > maxAIWorkflowFiles {
		files = files[:maxAIWorkflowFiles]
	}

	anyFailure := false
	for _, wf := range files {
		if ctx.Err() != nil {
			return true, anyFailure
		}
		content, ok := rawContent[wf.File]
		if !ok {
			continue
		}
		if err := reviewOneWorkflow(ctx, aiSvc, in, wf, content, firedLinesByFile[wf.File], emit); err != nil {
			anyFailure = true
		}
	}
	return true, anyFailure
}

func reviewOneWorkflow(ctx context.Context, aiSvc ai.Service, in domain.ScanInput, wf Workflow, content []byte, firedLines []int, emit func(domain.Finding)) error {
	source := domain.Location{Type: domain.LocationTypeFile, Path: wf.File, LineStart: wf.Line}

	result, err := aiSvc.Run(ctx, ai.RunInput{
		PromptID: ai.PromptReviewWorkflow,
		Vars: map[string]string{
			"already_fired_rule_ids": firedRuleIDsSummary(firedLines),
			"workflow_path":          wf.File,
		},
		Untrusted: []ai.UntrustedBlock{{Label: wf.File, Content: string(content)}},
		ScanID:    in.ScanID,
		Engine:    domain.EngineCICDScan,
		Source:    source,
	}, emit) // emit is also where ai.Service raises prompt_injection_attempt itself, on a discarded response
	if err != nil {
		return err
	}
	if result.Discarded {
		return nil // injection finding already emitted by ai.Service
	}

	findings, ok := result.Value.(ai.WorkflowReviewResponse)
	if !ok {
		return fmt.Errorf("cicdscan: unexpected AI result type %T", result.Value)
	}
	for _, f := range findings {
		if hint, ok := parseLineHint(f.LocationHint); ok && nearAny(hint, firedLines) {
			continue // §10: discard an AI finding that overlaps a rule finding within ±2 lines
		}
		emit(aiFinding(in.ScanID, wf, f))
	}
	return nil
}

// firedRuleIDsSummary is deliberately a plain description, not a literal
// rule-ID list keyed by line — the prompt only needs enough context to
// avoid repeating obviously-covered ground; the real, precise deduplication
// happens after the fact in reviewOneWorkflow via line proximity, not by
// trusting the model to honour the hint perfectly.
func firedRuleIDsSummary(firedLines []int) string {
	if len(firedLines) == 0 {
		return "(none yet)"
	}
	return fmt.Sprintf("%d deterministic rule findings already fired in this file", len(firedLines))
}

func parseLineHint(hint string) (int, bool) {
	m := digitsInHint.FindString(hint)
	if m == "" {
		return 0, false
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0, false
	}
	return n, true
}

func nearAny(line int, firedLines []int) bool {
	for _, fl := range firedLines {
		if abs(line-fl) <= lineNearMiss {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// aiFinding converts one AI-authored WorkflowReviewFinding into a
// domain.Finding — RuleID is always the single pre-registered
// cicdscan.ai.semantic-finding anchor (see rules.go's own doc comment on
// why an AI response can't propose its own rule_id at runtime), with the
// model's own suggested rule_id preserved in Metadata for transparency.
// Confidence is always Medium regardless of what the model implies — §10:
// "AI-only findings carry source: 'ai', confidence ≤ medium."
func aiFinding(scanID uuid.UUID, wf Workflow, f ai.WorkflowReviewFinding) domain.Finding {
	const ruleID = "cicdscan.ai.semantic-finding"
	meta := ruleMetaByID(ruleID)
	normLoc := wf.File + "|ai:" + strings.TrimSpace(f.LocationHint)

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCICDScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, f.Title),
		Title:       f.Title + " (" + wf.File + ")",
		Description: f.Description,
		Severity:    domain.Severity(f.Severity), Confidence: domain.ConfidenceMedium,
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: wf.File},
		Evidence:    []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: f.Excerpt}},
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Source:      domain.FindingSourceAI,
		Metadata:    map[string]any{"ai_suggested_rule_id": f.RuleID, "location_hint": f.LocationHint},
	}
}
