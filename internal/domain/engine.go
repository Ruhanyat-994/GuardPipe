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
	// CloneURL is the same URL orchestrator.Pool.prepareWorkspace's own
	// clone step already used (modules/project.Service.GetCloneInfo) — set
	// only by the ScanInput construction site that has it, currently just
	// engines that need to clone the repository themselves rather than
	// read the orchestrator's own already-cloned ScanInput.WorkspaceDir
	// (adapters/k8scodescanscanner, which has no shared filesystem with
	// guardpipe-worker to read that checkout from). Never assume this is
	// set — it wasn't, anywhere, before that adapter needed it.
	CloneURL string
}

// PentestTarget identifies the authorised, already-validated network target
// for the pentest engine. Target validation itself (DNS resolution, private/
// loopback rejection, attestation) lives in modules/project (Phase 3) — by
// the time an Engine sees a PentestTarget it has already passed that gate.
type PentestTarget struct {
	Host string
	IPs  []string
}

// DocumentRef is one document available to the docreview engine (Phase 11)
// — a design/requirements document the user uploaded directly to the
// project, not part of the git checkout. Path is a display name only (the
// uploaded filename), not a real filesystem path; full storage/metadata
// lives in modules/project. Documents discovered inside the repository
// checkout itself (WorkspaceDir) are a separate, secondary source the
// engine reads on its own — this field only ever carries the uploaded set.
type DocumentRef struct {
	Path    string
	Content string
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
	Documents    []DocumentRef
	Options      map[string]any // engine-specific, validated by the engine

	// ReportProgress lets an engine that has real, named stages of its own
	// (pentest's recon/service_id/tls/... phases, today) publish live "what
	// is actually happening right now" progress — pct in [0,100), a real
	// fact about how far through its own work it is, and activity, a short
	// human-readable label naming the stage ("Fuzzing for hidden paths and
	// files", not a generic "Running…"). Purely optional: an engine that
	// never calls this simply doesn't have named stages to report — the
	// orchestrator falls back to an elapsed-time-based estimate for it
	// instead of inventing fake per-stage detail (worker.go/service.go's
	// own doc comments explain that fallback).
	//
	// nil unless the caller sets it (worker.go always does; a ScanInput
	// built directly in a test literal does not) — an engine that wants to
	// call this must nil-check first, e.g. `if in.ReportProgress != nil { … }`.
	// Never call this with 100 — the orchestrator itself sets the terminal
	// 100% once the job actually finishes, so an engine reporting its own
	// "done" prematurely can't race that.
	ReportProgress func(pct int, activity string)
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
