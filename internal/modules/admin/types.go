// Package admin is the platform-operator control plane (BUILD_GUIDE.md
// Phase 14) — cross-organisation visibility and control for GuardPipe's own
// team, never for a tenant. This is deliberately not an extension of the
// existing per-organisation `admin` role (domain.RoleAdmin,
// documentation/02-srs.md FR-IAM-007): that role is scoped to one
// organisation by design, and this module's whole reason to exist is the
// narrow, fully-audited exception to that isolation a platform operator
// needs. See the 2026-08-31 note at the top of BUILD_GUIDE.md's Phase 14.
package admin

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Page is a 1-based page request, matching every other module's local copy
// of this shape (e.g. project.Page) rather than a shared type — each module
// stays free to evolve its own pagination independently.
type Page struct {
	Page     int
	PageSize int
}

// OrganizationSummary is one row of `GET /admin/organizations` — real
// per-org counts alongside the current suspension state.
type OrganizationSummary struct {
	ID              uuid.UUID
	Name            string
	MemberCount     int
	ProjectCount    int
	ScanCount       int
	SuspendedAt     *time.Time
	SuspendedReason *string
	CreatedAt       time.Time
}

// OrganizationDetail is `GET /admin/organizations/{id}` — the summary plus
// its member list, composed in the service layer from two repository reads
// (Service.GetOrganization), same shape project.Service.composeDetail
// already uses for ProjectDetail.
type OrganizationDetail struct {
	OrganizationSummary
	Members []UserSummary
}

// UserSummary is one row of `GET /admin/users` and one entry of an
// OrganizationDetail's Members.
type UserSummary struct {
	ID              uuid.UUID
	OrgID           uuid.UUID
	Email           string
	DisplayName     string
	Role            domain.Role
	SuspendedAt     *time.Time
	SuspendedReason *string
	LastLoginAt     *time.Time
	CreatedAt       time.Time
}

// OperatorGrant mirrors the `platform_operators` table
// (internal/store/migrations/00017_admin_platform_operators.sql) —
// provisioned only by the `guardpipe admin grant-operator` CLI subcommand,
// never by an HTTP call (see this package's Service doc comment).
type OperatorGrant struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	GrantedBy *uuid.UUID
	Note      string
	CreatedAt time.Time
}

// FlagStatus mirrors the `flag_status` Postgres enum (migration 00017).
type FlagStatus string

const (
	FlagOpen            FlagStatus = "open"
	FlagInvestigating   FlagStatus = "investigating"
	FlagDismissed       FlagStatus = "dismissed"
	FlagConfirmedMisuse FlagStatus = "confirmed_misuse"
)

func (s FlagStatus) Valid() bool {
	switch s {
	case FlagOpen, FlagInvestigating, FlagDismissed, FlagConfirmedMisuse:
		return true
	default:
		return false
	}
}

// FlagSource mirrors the `pentest_target_flags.source` CHECK constraint.
type FlagSource string

const (
	FlagSourceSelfReported      FlagSource = "self_reported"
	FlagSourceExternalComplaint FlagSource = "external_complaint"
	FlagSourceOperatorReview    FlagSource = "operator_review"
)

func (s FlagSource) Valid() bool {
	switch s {
	case FlagSourceSelfReported, FlagSourceExternalComplaint, FlagSourceOperatorReview:
		return true
	default:
		return false
	}
}

// PentestFlag mirrors the `pentest_target_flags` table.
type PentestFlag struct {
	ID         uuid.UUID
	TargetID   uuid.UUID
	Status     FlagStatus
	Source     FlagSource
	Reason     string
	ReportedBy *uuid.UUID
	ResolvedBy *uuid.UUID
	ResolvedAt *time.Time
	CreatedAt  time.Time
}

// TargetInfo is the display context a flag needs — which host, project, and
// organisation it points at — read across module boundaries the same way
// GetDisplayName already lets `project` read one field from `identity`'s
// tables: a read-only join, not a business operation, implemented directly
// in internal/store/repo against the tables `project` owns.
type TargetInfo struct {
	TargetID    uuid.UUID
	Host        string
	ProjectID   uuid.UUID
	ProjectName string
	OrgID       uuid.UUID
	OrgName     string
}

// PentestFlagDetail is one row of `GET /admin/pentest-flags` — the flag plus
// the target context an operator needs to triage it without a second
// round trip.
type PentestFlagDetail struct {
	PentestFlag
	Target TargetInfo
}

// CreateFlagInput is Service.CreateFlag's input
// (`POST /admin/pentest-flags` — reachable by any authenticated user
// reporting misuse of a target that scanned infrastructure they own, not
// operator-gated for creation, only for triage).
type CreateFlagInput struct {
	TargetID uuid.UUID
	Source   FlagSource
	Reason   string
}

// ResolveFlagInput is Service.ResolveFlag's input
// (`PATCH /admin/pentest-flags/{id}`, operator-only).
type ResolveFlagInput struct {
	Status FlagStatus
}

// EngineJobStats is one engine's job outcome counts over the health
// window (Service.SystemHealth).
type EngineJobStats struct {
	Engine    domain.EngineID
	Succeeded int
	Failed    int
	Skipped   int
}

// GeminiPoolStatus reports the multi-key rotation pool's shape without ever
// exposing a key value (adapters/gemini.KeyPool.Current() returns the raw
// secret — never surfaced here). Available is false when AI is disabled —
// distinct from an empty pool, which can't happen while AI is enabled
// (config.Load already requires at least one key in that case).
type GeminiPoolStatus struct {
	Available    bool
	PoolSize     int
	CurrentIndex int
}

// AICacheStatus reports the in-process content-hash cache's hit rate
// (BUILD_GUIDE.md Phase 4, `documentation/10-ai-integration.md` §7).
// Available is false when AI is disabled.
type AICacheStatus struct {
	Available bool
	Hits      int
	Misses    int
}

// SystemHealth is `GET /admin/system-health`. Every field that can't be
// read cheaply and honestly is reported as unavailable rather than
// fabricated — the same "never fake a number" rule AiPanel's
// unavailable/budget-exhausted states and PartialResultBanner already
// follow elsewhere in this product.
type SystemHealth struct {
	// EngineStats covers the last 24 hours, from scan_jobs — always
	// available, no adapter dependency.
	EngineStats []EngineJobStats
	// JobsInFlight is the count of scan jobs currently claimed by a worker
	// (adapters/queue.JobQueue.ListProcessing) — "in flight," not the full
	// Redis stream's pending depth, named precisely so the number isn't
	// read as something it isn't.
	JobsInFlight int
	// SandboxContainersRunning is nil when it couldn't be read (no Docker
	// daemon reachable) rather than reported as zero.
	SandboxContainersRunning *int
	Gemini                   GeminiPoolStatus
	AICache                  AICacheStatus
	CheckedAt                time.Time
}
