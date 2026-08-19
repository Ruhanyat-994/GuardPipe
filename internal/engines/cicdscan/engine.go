package cicdscan

import (
	"context"
	"fmt"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

// Engine implements domain.Engine — cicdscan's GitHub Actions workflow
// discovery, 16 deterministic Core rules, and (documentation/05-module-specifications.md
// §10's own ordering) an AI semantic pass layered on top. aiSvc may be nil
// (GUARDPIPE_AI_ENABLED=false, or no Gemini key configured at wiring time)
// — Run degrades to rule findings only in that case, per §10's own
// "Gemini unavailable" failure mode; it never fails the job.
type Engine struct {
	aiSvc ai.Service
}

func New(aiSvc ai.Service) *Engine {
	return &Engine{aiSvc: aiSvc}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineCICDScan
}

// Applicable is a cheap existence check: does `.github/workflows/` contain
// anything at all — documentation/05-module-specifications.md §10's own
// failure-mode table: "No .github/workflows/: Applicable false -> skipped".
func (e *Engine) Applicable(ctx context.Context, in domain.ScanInput) (bool, string) {
	discovery, err := discoverWorkflows(in.WorkspaceDir)
	if err != nil {
		return false, "could not read .github/workflows/"
	}
	if len(discovery.Workflows) > 0 || len(discovery.ParseErrors) > 0 {
		return true, ""
	}
	return false, "no GitHub Actions workflows found in .github/workflows/"
}

// Run discovers every workflow, evaluates all 16 Core rules against each,
// then — deterministic rules first, AI strictly as a supplement — runs one
// AI semantic pass per file. Never writes to the database; the orchestrator
// persists whatever Run emits (documentation/03-architecture-overview.md
// §6.3).
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	discovery, err := discoverWorkflows(in.WorkspaceDir)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("cicdscan: discover workflows: %w", err)
	}

	var skipped []domain.SkipReason
	for file, parseErr := range discovery.ParseErrors {
		skipped = append(skipped, domain.SkipReason{RuleID: "cicdscan.*", Reason: "parse_error: " + file + ": " + parseErr.Error()})
	}

	firedLinesByFile := map[string][]int{}
	for _, wf := range discovery.Workflows {
		if ctx.Err() != nil {
			return domain.EngineResult{}, ctx.Err()
		}
		var fileFindings []domain.Finding
		fileFindings = append(fileFindings, evaluateSupplyChain(in.ScanID, wf)...)
		fileFindings = append(fileFindings, evaluateTriggers(in.ScanID, wf)...)
		fileFindings = append(fileFindings, evaluatePermissions(in.ScanID, wf)...)
		fileFindings = append(fileFindings, evaluateSecrets(in.ScanID, wf)...)
		fileFindings = append(fileFindings, evaluateRunner(in.ScanID, wf)...)

		for _, f := range fileFindings {
			emit(f)
			if f.Location.LineStart > 0 {
				firedLinesByFile[wf.File] = append(firedLinesByFile[wf.File], f.Location.LineStart)
			}
		}
	}

	aiRan, aiUnavailable := runAIPass(ctx, e.aiSvc, in, discovery.Workflows, discovery.RawContent, firedLinesByFile, emit)
	if !aiRan {
		skipped = append(skipped, domain.SkipReason{RuleID: "cicdscan.ai.*", Reason: "ai_pass_unavailable"})
	}

	return domain.EngineResult{
		RulesEvaluated: len(Rules),
		FilesScanned:   len(discovery.Workflows),
		Skipped:        skipped,
		Stats: map[string]any{
			"workflows_found":  len(discovery.Workflows),
			"ai_pass_ran":      aiRan,
			"ai_pass_degraded": aiUnavailable,
		},
	}, nil
}
