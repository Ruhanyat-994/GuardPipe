package ai

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// errUnexpectedResponseType/errDiscarded are internal sentinels enrichOne
// treats identically to a budget/provider failure — nothing to persist,
// nothing to surface as an error to EnrichScan's caller (§9: an AI call
// not working out never fails the scan).
var (
	errUnexpectedResponseType = errors.New("ai: response had an unexpected type")
	errDiscarded              = errors.New("ai: response discarded (suspected prompt injection)")
)

// EnrichmentLevel says how much AI enrichment a finding earns, before any
// budget is spent — documentation/10-ai-integration.md §8's priority
// order, encoded once so both the bulk post-scan path and any future
// on-demand path agree on it.
type EnrichmentLevel int

const (
	EnrichNone EnrichmentLevel = iota
	EnrichExplanationOnly
	EnrichExplanationAndPatch
)

// LevelForSeverity implements §8's exact table: critical/high get an
// explanation and a patch, medium gets an explanation only, low/
// informational get nothing automatically (still available on demand,
// which is a separate, not-yet-built endpoint — see this package's
// package doc).
func LevelForSeverity(sev domain.Severity) EnrichmentLevel {
	switch sev {
	case domain.SeverityCritical, domain.SeverityHigh:
		return EnrichExplanationAndPatch
	case domain.SeverityMedium:
		return EnrichExplanationOnly
	default:
		return EnrichNone
	}
}

// Suggestion is what gets persisted to `ai_suggestions` for one finding —
// documentation/06-database-design.md §4.13's columns, one row per
// finding.
type Suggestion struct {
	FindingID   uuid.UUID
	Explanation string
	PatchDiff   string
	// PatchStatus is "unverified" whenever PatchDiff is non-empty, never
	// "verified" — verifying a patch needs a sandboxed checkout
	// (`git apply --check`), which no longer exists by the time enrichment
	// runs (the workspace is released once every job for the scan is
	// done — see worker.go's releaseWorkspace). "not_applicable" when no
	// patch was generated at all (a medium-severity explanation-only
	// suggestion, or a patch call that itself got skipped/failed).
	PatchStatus   string
	Model         string
	PromptVersion string
	InputHash     string
	TokensIn      int
	TokensOut     int
	GeneratedAt   time.Time
}

// SuggestionRepository persists one AI suggestion per finding — defined
// here (the consumer), implemented by store/repo.AISuggestionRepo.
type SuggestionRepository interface {
	Upsert(ctx context.Context, s Suggestion) error
}

// EnrichScanResult summarises one EnrichScan call for logging — not
// surfaced to any API response. SkippedForBudget and Failed are counted
// separately: the first is the expected, documented "we chose not to
// spend here" case (§8: "exhaustion is not an error"), the second is a
// genuine problem (the provider call errored, returned something
// unusable, or persistence failed) — collapsing them into one counter
// would make a real outage (e.g. every call 404ing on a misconfigured
// model name, or the provider's quota running out) silently look like
// ordinary budget management in the logs.
type EnrichScanResult struct {
	Enriched         int
	SkippedForBudget int
	Failed           int
}

// Enricher produces and persists AI suggestions for findings, respecting a
// per-scan token budget (§8). Shared shape for both the bulk post-scan
// path (orchestrator/worker.go's Pool.enrichFindings, priority-ordered
// across every finding in one scan) and any future on-demand single-
// finding path (POST /findings/{id}/explain, documented in §8's own
// priority-order table but not yet built — see this package's package
// doc for what Phase 13 scoped in vs. out).
type Enricher struct {
	svc    Service
	budget BudgetTracker
	store  SuggestionRepository
	log    *slog.Logger
}

// NewEnricher wires an Enricher. All three dependencies are required —
// callers that don't want AI enrichment at all (GUARDPIPE_AI_ENABLED=false)
// simply don't construct one and leave orchestrator.Pool.Enricher nil,
// the same nil-is-disabled convention every other optional AI-consuming
// dependency in this codebase already follows.
func NewEnricher(svc Service, budget BudgetTracker, store SuggestionRepository, log *slog.Logger) *Enricher {
	if log == nil {
		log = slog.Default()
	}
	return &Enricher{svc: svc, budget: budget, store: store, log: log}
}

// EnrichScan enriches scanID's findings in priority order — every critical
// first, then every high, then every medium — spending the budget on the
// findings that matter most before it might run out partway through
// (§8: "When the budget cannot cover everything, spend it where it
// matters"). low/informational findings are never touched here at all.
func (e *Enricher) EnrichScan(ctx context.Context, scanID uuid.UUID, findings []domain.Finding) EnrichScanResult {
	ordered := prioritise(findings)
	result := EnrichScanResult{}
	for _, f := range ordered {
		level := LevelForSeverity(f.Severity)
		if level == EnrichNone {
			continue
		}
		switch err := e.enrichOne(ctx, scanID, f, level); {
		case err == nil:
			result.Enriched++
		case errors.Is(err, ErrBudgetExhausted):
			result.SkippedForBudget++
		default:
			result.Failed++
		}
	}
	return result
}

// prioritise returns only the findings worth enriching at all (severity
// rank 0-2: critical/high/medium), sorted worst-first — a stable sort so
// findings of equal severity keep their input order, avoiding a
// non-deterministic pick of "which mediums got the last of the budget"
// across otherwise-identical runs.
func prioritise(findings []domain.Finding) []domain.Finding {
	out := make([]domain.Finding, 0, len(findings))
	for _, f := range findings {
		if LevelForSeverity(f.Severity) != EnrichNone {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity.Rank() < out[j].Severity.Rank() })
	return out
}

func (e *Enricher) enrichOne(ctx context.Context, scanID uuid.UUID, f domain.Finding, level EnrichmentLevel) error {
	explanation, err := e.explain(ctx, scanID, f)
	if err != nil {
		// Budget exhausted or the call genuinely failed — either way,
		// nothing to persist (doc §9: the finding just shows its
		// deterministic remediation instead). The caller (EnrichScan)
		// distinguishes which one this was via errors.Is(err, ErrBudgetExhausted).
		return err
	}

	suggestion := Suggestion{
		FindingID: f.ID, Explanation: explanation.text, PatchStatus: "not_applicable",
		Model: explanation.model, PromptVersion: PromptVersion(PromptExplainFinding), InputHash: explanation.inputHash,
		TokensIn: explanation.tokensIn, TokensOut: explanation.tokensOut, GeneratedAt: time.Now().UTC(),
	}

	if level == EnrichExplanationAndPatch {
		if patch, err := e.patch(ctx, scanID, f); err == nil {
			suggestion.PatchDiff = patch.text
			suggestion.PatchStatus = "unverified"
			suggestion.Model = patch.model // the more specific of the two calls' models — patch uses ModelTierSmart, a stronger model than the explanation's ModelTierFast, worth recording over the weaker one when both ran
			suggestion.TokensIn += patch.tokensIn
			suggestion.TokensOut += patch.tokensOut
		}
		// A failed/skipped patch call still leaves the explanation-only
		// suggestion above worth persisting — partial enrichment beats none.
	}

	if err := e.store.Upsert(ctx, suggestion); err != nil {
		e.log.Error("ai: persist suggestion failed", "finding_id", f.ID, "error", err)
		return err
	}
	return nil
}

// callResult is the common shape explain/patch reduce a Service.Run call
// to — just what enrichOne needs to build a Suggestion.
type callResult struct {
	text                string
	model, inputHash    string
	tokensIn, tokensOut int
}

func (e *Enricher) explain(ctx context.Context, scanID uuid.UUID, f domain.Finding) (callResult, error) {
	vars := map[string]string{
		"rule_id": f.RuleID, "title": f.Title, "severity": string(f.Severity),
		"cwe": strings.Join(f.CWE, ", "), "location": locationSummary(f.Location),
	}
	result, err := e.run(ctx, scanID, PromptExplainFinding, vars, f)
	if err != nil {
		return callResult{}, err
	}
	resp, ok := result.Value.(ExplainFindingResponse)
	if !ok {
		e.log.Warn("ai: explain_finding response had an unexpected type", "finding_id", f.ID)
		return callResult{}, errUnexpectedResponseType
	}
	text := resp.What + " " + resp.WhyItMatters
	if resp.HowExploited != "" {
		text += " " + resp.HowExploited
	}
	return callResult{
		text: text, model: result.Model, tokensIn: result.TokensIn, tokensOut: result.TokensOut,
		inputHash: cacheKeyOrEmpty(PromptExplainFinding, result.Model, vars),
	}, nil
}

func (e *Enricher) patch(ctx context.Context, scanID uuid.UUID, f domain.Finding) (callResult, error) {
	vars := map[string]string{
		"rule_id": f.RuleID, "title": f.Title, "file_path": f.Location.Path,
		"language": languageForPath(f.Location.Path), "remediation": f.Remediation,
	}
	result, err := e.run(ctx, scanID, PromptGeneratePatch, vars, f)
	if err != nil {
		return callResult{}, err
	}
	resp, ok := result.Value.(GeneratePatchResponse)
	if !ok {
		e.log.Warn("ai: generate_patch response had an unexpected type", "finding_id", f.ID)
		return callResult{}, errUnexpectedResponseType
	}
	return callResult{
		text: resp.Patch, model: result.Model, tokensIn: result.TokensIn, tokensOut: result.TokensOut,
		inputHash: cacheKeyOrEmpty(PromptGeneratePatch, result.Model, vars),
	}, nil
}

// run is explain/patch's shared reserve-call-release sequence: reserve the
// prompt's MaxTokens as a pessimistic estimate before calling (the real
// cost isn't known until the response comes back), then release it again
// if the call turned out to be free (served from cache) or failed outright
// — a budget spend should reflect real usage as closely as this two-step
// reserve/release approximation allows, not just "we thought about calling
// it."
func (e *Enricher) run(ctx context.Context, scanID uuid.UUID, promptID PromptID, vars map[string]string, f domain.Finding) (*RunResult, error) {
	amount := PromptMaxTokens(promptID)
	if err := e.budget.Reserve(ctx, scanID, amount); err != nil {
		e.log.Info("ai: enrichment skipped, budget exhausted", "finding_id", f.ID, "prompt", promptID, "scan_id", scanID)
		return nil, err
	}

	// emit is nil: neither explain_finding nor generate_patch ever sends
	// Untrusted content (both prompts' own Vars are deterministic, already-
	// in-the-database strings — rule_id/title/severity/location/remediation
	// — never raw repository text), so ErrInjectionSuspected discarding a
	// response here is not a realistic path the way it is for an engine's
	// AI pass over real file/document content. If Service.Run ever discards
	// a call here regardless, that's still handled below (Discarded is
	// checked the same as any other unusable result) — just without the
	// standalone-finding emission an engine's own job-transaction context
	// would provide, since no such context exists this long after every
	// job for the scan already finished.
	result, err := e.svc.Run(ctx, RunInput{PromptID: promptID, Vars: vars, ScanID: scanID, Engine: f.Engine, Source: f.Location}, nil)
	if err != nil {
		e.budget.Release(ctx, scanID, amount)
		e.log.Warn("ai: enrichment call failed", "finding_id", f.ID, "prompt", promptID, "error", err)
		return nil, err
	}
	if result.Discarded {
		e.budget.Release(ctx, scanID, amount)
		return nil, errDiscarded
	}
	if result.FromCache {
		e.budget.Release(ctx, scanID, amount) // cost nothing real — give the reservation back
	}
	return result, nil
}

func cacheKeyOrEmpty(promptID PromptID, model string, vars map[string]string) string {
	key, err := CacheKey(promptID, PromptVersion(promptID), model, vars, nil)
	if err != nil {
		return ""
	}
	return key
}

// languageForPath is a small, deliberately incomplete extension map — good
// enough to give generate_patch's {{.language}} var something useful for
// the languages GuardPipe's own engines actually scan; an unrecognised
// extension just leaves it empty, which the prompt handles fine (it's
// context for the model, not a required field).
var languageByExtension = map[string]string{
	".go": "Go", ".py": "Python", ".js": "JavaScript", ".ts": "TypeScript", ".tsx": "TypeScript",
	".java": "Java", ".rb": "Ruby", ".php": "PHP", ".yaml": "YAML", ".yml": "YAML",
	".tf": "Terraform", ".dockerfile": "Dockerfile",
}

func languageForPath(path string) string {
	return languageByExtension[strings.ToLower(filepath.Ext(path))]
}

// locationSummary is a short, human-readable rendering of a Location for
// the explain_finding prompt's {{.location}} var — not the full JSONB
// shape (documentation/06-database-design.md §5), just enough for the
// model to reference where the issue is.
func locationSummary(l domain.Location) string {
	switch l.Type {
	case domain.LocationTypeFile:
		if l.LineStart > 0 {
			return l.Path + ":" + strconv.Itoa(l.LineStart)
		}
		return l.Path
	case domain.LocationTypeImage:
		return l.Image
	case domain.LocationTypeK8s:
		return l.Kind + "/" + l.Name
	case domain.LocationTypeNetwork:
		return l.Host
	case domain.LocationTypeDependency:
		return l.Package + "@" + l.Version
	default:
		return ""
	}
}
