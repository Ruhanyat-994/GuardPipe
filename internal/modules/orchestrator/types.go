// Package orchestrator is the minimal version BUILD_GUIDE.md Phase 6 scopes:
// scan creation, the Redis job queue, the worker pool, and an engine
// registry wired to depscan only. Findings persistence (one transaction
// per completed job) is this phase's job too; populating the `dependencies`
// inventory table migration 00009 also created is not — Phase 6's own
// bullet list names only "Findings persistence," not dependency-inventory
// persistence, so that table stays empty until a phase that actually reads
// it (an SBOM export, say) needs it populated.
package orchestrator

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/scoring"
)

// CreateScanInput matches `POST /projects/{id}/scans`
// (documentation/07-api-specification.md §5). Engines is only meaningful
// when Type is ScanTypePartial; a full_supply_chain scan runs every engine
// currently registered (today, just depscan — the registry decides, not a
// hardcoded list of seven engines this phase doesn't build).
type CreateScanInput struct {
	Type    domain.ScanType
	Engines []domain.EngineID
	Branch  string
	// PentestConfig is optional — nil means "use the server-side default"
	// (Stealth, literally, not just a UI suggestion — BUILD_GUIDE.md Phase
	// 12). Ignored when the resolved engine set doesn't include pentest.
	PentestConfig *domain.PentestScanConfig
	// SourceIP is the requesting client's IP, captured by the handler
	// (c.ClientIP()) the same way dto.AttestTargetRequest.ToInput already
	// does for target.attested — persisted onto the scan row and surfaced by
	// reporting.Assembler as the exported report's accountability watermark
	// (who ran this scan, and from where).
	SourceIP string
	// TriggerSource/TriggerRef/TriggerActor record where the scan came from
	// (domain.Scan's fields of the same names). An empty TriggerSource means
	// manual — the HTTP handler never sets it, only the scheduler and the
	// live-scanning webhook worker do.
	TriggerSource domain.TriggerSource
	TriggerRef    string
	TriggerActor  string
}

// TokenCharger is billing's side of scan creation, defined here (the
// consumer) so orchestrator never imports modules/billing. Every scan —
// manual, scheduled, webhook, CLI — goes through CreateScan, so this is
// the one place a scan can't avoid being paid for. nil disables billing
// (GUARDPIPE_BILLING_MODE=off, and every existing test).
type TokenCharger interface {
	// Charge pays for all of a scan's jobs, or none. The price is computed
	// by billing from the engines, preset and trigger. Returns
	// billing.plan_required (403) or billing.insufficient_tokens (402).
	Charge(ctx context.Context, orgID uuid.UUID, req ChargeRequest) error
	// RefundJob returns what one job was charged; a no-op if it never was.
	RefundJob(ctx context.Context, jobID uuid.UUID, reason string) error
	// AllowedEngines filters engines to what the org's plan may run, so an
	// "everything available" scan quietly leaves out what the plan lacks.
	AllowedEngines(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID) ([]domain.EngineID, error)
	// RequireFeature returns billing.plan_required if the org's plan lacks
	// feature ("schedules").
	RequireFeature(ctx context.Context, orgID uuid.UUID, feature string) error
}

// ChargeRequest is one scan's charge. Lines are the jobs about to be created.
type ChargeRequest struct {
	ScanID  uuid.UUID
	Lines   []ChargeLine
	Preset  domain.PentestPreset
	Trigger domain.TriggerSource
	ActorID uuid.UUID
}

type ChargeLine struct {
	JobID  uuid.UUID
	Engine domain.EngineID
}

// ScanDetail is a Scan plus its jobs — documentation/07-api-specification.md
// §5's `GET /scans/{id}` shape (risk/AI fields are Phase 13's, omitted
// here rather than faked).
type ScanDetail struct {
	domain.Scan
	Jobs []JobDetail
	// PentestConfigClamped names any PentestConfig field CreateScan reduced
	// to fit the server-side ceiling ("scan speed capped at 10 req/s") —
	// populated only by CreateScan's own response, not persisted or
	// returned by GetScan later (BUILD_GUIDE.md Phase 12).
	PentestConfigClamped []string
	// Risk is nil until the scan's last job finalizes and Pool.finalizeScoring
	// (worker.go) persists a RiskAssessmentRecord for it — a queued or
	// running scan, or one that finished before scoring existed, has no
	// score yet, which is a real state the API and UI must both represent
	// honestly rather than fabricate a number for.
	Risk *RiskAssessmentRecord
}

// OrgScanSummary is one row of the org-wide scan history (`GET /scans`) —
// a Scan (already carrying ProjectID) plus the project's name, so a
// cross-project list can render "Project · Scan" without a second
// round-trip per row.
type OrgScanSummary struct {
	domain.Scan
	ProjectName string
}

// JobDetail is a ScanJob plus its finding count — the list item in
// ScanDetail.Jobs.
type JobDetail struct {
	domain.ScanJob
	FindingCount int
}

// EngineProgress is one row of the progress-polling response
// (documentation/07-api-specification.md §5's `GET /scans/{id}/progress`).
type EngineProgress struct {
	Engine      domain.EngineID
	Status      domain.JobStatus
	ProgressPct int
	// Activity is a real, live "what's happening right now" label — an
	// engine's own reported stage (pentest's actual phase names, e.g.
	// "Fuzzing for hidden paths and files") when one exists, empty
	// otherwise. Empty is a normal value, not a bug: the frontend already
	// has a generic per-engine fallback label (lib/engines.ts) for engines
	// that don't report named stages.
	Activity     string
	FindingCount int
}

// RiskAssessmentRecord is a scoring.RiskAssessment plus the scan-context
// fields only the orchestrator (not the pure scoring package) can know:
// which scan it's for, and the previous score for the delta
// (documentation/11-risk-scoring-and-severity.md §6) — scoring.Compute has
// no access to other scans, so that lookup happens here, one layer up.
type RiskAssessmentRecord struct {
	ScanID        uuid.UUID
	Score         int
	Verdict       domain.Verdict
	EngineScores  map[domain.EngineID]int
	Breakdown     []scoring.Contribution
	PreviousScore *int
	// Delta is Score - PreviousScore, computed at read time (GetScan) rather
	// than stored — negative means improving, positive means regressing
	// (documentation/11-risk-scoring-and-severity.md §6). nil whenever
	// PreviousScore is nil (no prior completed scan to compare against).
	Delta          *int
	IsPartial      bool
	FormulaVersion string
	ComputedAt     time.Time
}

// Progress is the whole polling response — deliberately tiny, served from
// Redis in the fuller design (documentation/07-api-specification.md: "a
// 2-second poll must not hit the database"). This phase serves it from
// PostgreSQL directly (job/finding counts) — a documented simplification,
// not the Redis-backed `gp:progress:{scan_id}` hash the spec describes;
// revisit once poll volume actually matters.
type Progress struct {
	ScanID      uuid.UUID
	Status      domain.ScanStatus
	ProgressPct int
	Engines     []EngineProgress
	UpdatedAt   time.Time
}
