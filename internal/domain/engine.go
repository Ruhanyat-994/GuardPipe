package domain

import (
	"context"

	"github.com/google/uuid"
)

// EngineID is the stable identifier of one of the seven security engines.
// Values match the `engine_id` Postgres enum (documentation/06-database-design.md
// §3) exactly, in the same order.
type EngineID string

const (
	EngineDocReview     EngineID = "docreview"
	EngineCodeScan      EngineID = "codescan"
	EngineDepScan       EngineID = "depscan"
	EngineContainerScan EngineID = "containerscan"
	EngineK8sScan       EngineID = "k8sscan"
	EngineCICDScan      EngineID = "cicdscan"
	EnginePentest       EngineID = "pentest"
)

func (id EngineID) Valid() bool {
	switch id {
	case EngineDocReview, EngineCodeScan, EngineDepScan, EngineContainerScan,
		EngineK8sScan, EngineCICDScan, EnginePentest:
		return true
	default:
		return false
	}
}

func (id EngineID) String() string {
	return string(id)
}

// RepositoryRef identifies the source-control checkout an engine is running
// against. Full repository/credential handling lives in modules/project
// (Phase 3) — this is just the addressing information an engine needs.
type RepositoryRef struct {
	Owner     string
	Name      string
	Branch    string
	CommitSHA string
}

// PentestTarget identifies the authorised, already-validated network target
// for the pentest engine. Target validation itself (DNS resolution, private/
// loopback rejection, attestation) lives in modules/project (Phase 3) — by
// the time an Engine sees a PentestTarget it has already passed that gate.
type PentestTarget struct {
	Host string
	IPs  []string
}

// ScanInput is everything an Engine needs to do its job, and nothing more —
// no database handle, no HTTP context. This is what keeps engines trivially
// unit-testable in isolation (documentation/05-module-specifications.md §2.1).
type ScanInput struct {
	ScanID       uuid.UUID
	JobID        uuid.UUID
	ProjectID    uuid.UUID
	WorkspaceDir string // ephemeral checkout root; read-only to engines
	Repository   *RepositoryRef
	Target       *PentestTarget
	Options      map[string]any // engine-specific, validated by the engine
}

// SkipReason names one rule that an engine chose not to evaluate and why —
// e.g. a rule that needs network access inside a no-network sandbox. This is
// distinct from Applicable returning false: Applicable skips the whole
// engine, SkipReason records partial skips within a run that still produced
// a result.
type SkipReason struct {
	RuleID string
	Reason string
}

// EngineResult summarises one engine's run for the UI's per-engine stage
// card. It carries no findings — those are streamed via the emit callback.
type EngineResult struct {
	RulesEvaluated int
	FilesScanned   int
	Skipped        []SkipReason
	Stats          map[string]any // shown in the UI engine card
}

// Engine is the one interface every one of the seven security engines
// implements, and the only thing the orchestrator depends on
// (documentation/03-architecture-overview.md §6.3). Adding an engine means
// implementing this interface and registering it — no orchestrator changes.
type Engine interface {
	// ID is this engine's stable identifier.
	ID() EngineID

	// Applicable is a cheap check: does this target have anything for this
	// engine to look at? Returning false marks the job "skipped", not
	// "failed" — e.g. containerscan on a repository with no Dockerfile.
	Applicable(ctx context.Context, in ScanInput) (bool, string)

	// Run does the work. It must respect ctx cancellation and deadline, must
	// never write to the database (the orchestrator persists findings, in
	// one transaction per job), and must not panic — the worker recovers
	// regardless, but a well-behaved engine doesn't rely on that.
	// emit streams findings to the orchestrator as they're found, so the UI
	// can show progress before the engine finishes.
	Run(ctx context.Context, in ScanInput, emit func(Finding)) (EngineResult, error)
}
