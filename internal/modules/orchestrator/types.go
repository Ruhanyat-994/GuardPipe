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
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
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
}

// ScanDetail is a Scan plus its jobs — documentation/07-api-specification.md
// §5's `GET /scans/{id}` shape (risk/AI fields are Phase 13's, omitted
// here rather than faked).
type ScanDetail struct {
	domain.Scan
	Jobs []JobDetail
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
	Engine       domain.EngineID
	Status       domain.JobStatus
	ProgressPct  int
	FindingCount int
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
