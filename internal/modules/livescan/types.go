// Package livescan is GitHub webhook live scanning (BUILD_GUIDE.md Phase 17
// Part B): a push or pull request on a watched branch automatically starts
// a scan, with the engines the user chose when turning it on.
//
// It's its own module rather than part of `project` (where BUILD_GUIDE.md
// first sketched it) because it has to call orchestrator.CreateScan, and
// orchestrator already imports project — putting this in project would be
// an import cycle. It depends on both through narrow interfaces, the same
// way orchestrator depends on project.
//
// Two rules this package enforces no matter what is stored or requested:
//   - penetration tests NEVER run automatically (AllowedEngines excludes
//     pentest; the check runs at enable time and again right before every
//     scan is created, and migration 00027 has a CHECK constraint too), and
//   - every automatic scan is attributed to the person who turned live
//     scanning on and ticked the confirmation (Webhook.EnabledBy), which is
//     what the exported report's "Authorisation & responsibility" section
//     then names.
package livescan

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

// AllowedEngines is every engine a push may start. pentest is deliberately
// absent: a push must never send live attack traffic at a target without a
// person deciding to, in that moment.
var AllowedEngines = []domain.EngineID{
	domain.EngineCodeScan,
	domain.EngineDepScan,
	domain.EngineContainerScan,
	domain.EngineK8sScan,
	domain.EngineCICDScan,
	domain.EngineDocReview,
}

// maxWatchedBranches bounds how many branches one project can watch.
const maxWatchedBranches = 20

// Webhook mirrors one `project_webhooks` row (migration 00027). A row
// existing means live scanning is on for that project.
type Webhook struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	GitHubHookID     int64
	SecretCiphertext []byte
	SecretNonce      []byte
	Engines          []domain.EngineID
	WatchedBranches  []string
	// EnabledBy/AttestedAt are the confirmation ("testimony"): who
	// authorised automatic scanning, and when.
	EnabledBy          *uuid.UUID
	AttestedAt         time.Time
	LastDeliveryAt     *time.Time
	LastDeliveryStatus string
	// MinBalancePercent: live scans stop once paying for one would leave
	// less than this percentage of the plan's monthly token grant, so
	// automation can't use up the tokens a person needs for a manual scan.
	MinBalancePercent int
	// PausedReason is set by the circuit breaker; nil means active.
	PausedReason *string
	PausedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// EnableInput is `PUT /projects/{id}/live-scanning`.
type EnableInput struct {
	Engines         []domain.EngineID
	WatchedBranches []string
	// MinBalancePercent is Webhook.MinBalancePercent; nil keeps the
	// current value (10 for a new webhook).
	MinBalancePercent *int
	// Confirmed is the confirmation checkbox. Required every time live
	// scanning is turned on or its settings change — including resuming it
	// after the circuit breaker paused it.
	Confirmed bool
}

// DisableResult says whether the hook was also removed from GitHub. Live
// scanning is turned off in GuardPipe either way; if GitHub refused the
// delete (token revoked, say) the user has to remove it there by hand, and
// any deliveries it still sends are rejected as unknown.
type DisableResult struct {
	GitHubHookRemoved bool
}

// Delivery is one incoming webhook request, exactly as received.
type Delivery struct {
	WebhookID  uuid.UUID
	Event      string // X-GitHub-Event
	DeliveryID string // X-GitHub-Delivery
	Signature  string // X-Hub-Signature-256
	Body       []byte
}

// Config is this module's settings, from platform/config.
type Config struct {
	// PublicBaseURL is the externally-reachable origin GitHub delivers to —
	// the ALB's HTTPS URL on AWS, or a path-preserving tunnel (cloudflared,
	// ngrok) for local testing.
	PublicBaseURL string
	EncryptionKey []byte
	// MaxScansPerProjectPerHour is the per-project cap. Billing's
	// entitlement.Check("live_scan") is meant to replace this later.
	MaxScansPerProjectPerHour int
	// Debounce is how long a push waits for more pushes to the same branch
	// before its scan starts.
	Debounce time.Duration
}

// breakerFactor: once a project hits this many times its hourly cap in
// trigger attempts, live scanning pauses itself until someone re-confirms.
const breakerFactor = 3

// deliverySeenTTL is how long a delivery ID is remembered for de-duplication.
const deliverySeenTTL = 24 * time.Hour

// Repository is defined by this package; implementation lives in
// internal/store/repo.
type Repository interface {
	Create(ctx context.Context, w *Webhook) error
	GetByID(ctx context.Context, id uuid.UUID) (*Webhook, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID) (*Webhook, error)
	// UpdateConfig writes Engines, WatchedBranches, EnabledBy, AttestedAt and
	// clears the pause.
	UpdateConfig(ctx context.Context, w *Webhook) error
	Delete(ctx context.Context, id uuid.UUID) error
	RecordDelivery(ctx context.Context, id uuid.UUID, at time.Time, status string) error
	Pause(ctx context.Context, id uuid.UUID, reason string, at time.Time) error
}

// HookClient is the subset of adapters/github.Client this package needs.
type HookClient interface {
	CreateHook(ctx context.Context, owner, name, token, url, secret string, events []string) (int64, error)
	DeleteHook(ctx context.Context, owner, name, token string, hookID int64) error
}

// ProjectAccess is the subset of project.Service this package needs.
type ProjectAccess interface {
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (*project.ProjectDetail, error)
	GetCloneInfo(ctx context.Context, projectID uuid.UUID) (repoURL, branch, token string, err error)
	GetOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)
}

// ScanCreator is the subset of orchestrator.Service this package needs.
type ScanCreator interface {
	CreateScan(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in orchestrator.CreateScanInput) (*orchestrator.ScanDetail, error)
}

// TokenGate is billing's side of live scanning (implemented by
// billing.Service). nil disables token checks (GUARDPIPE_BILLING_MODE=off).
type TokenGate interface {
	// RequireFeature returns billing.plan_required if the org's plan
	// doesn't include feature ("live_scanning").
	RequireFeature(ctx context.Context, orgID uuid.UUID, feature string) error
	// LiveScanAllowed: the plan includes live scanning and, after paying
	// for this scan, the balance stays at or above floorPercent of the
	// monthly grant. reason is "plan_required" or "insufficient_tokens".
	LiveScanAllowed(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID, floorPercent int) (ok bool, reason string, err error)
}

// defaultMinBalancePercent is the live-scan token floor for a new webhook.
const defaultMinBalancePercent = 10

// commitSeenTTL is how long a scanned commit is remembered, so the same
// commit (a re-delivery, or a PR opened on an already-pushed commit) isn't
// paid for twice.
const commitSeenTTL = 24 * time.Hour

// Coordinator is the Redis-backed half (adapters/queue.LiveScanStore).
type Coordinator interface {
	MarkDeliverySeen(ctx context.Context, deliveryID string, ttl time.Duration) (bool, error)
	PushEvent(ctx context.Context, payload []byte) error
	PopEvent(ctx context.Context, timeout time.Duration) ([]byte, error)
	ArmPending(ctx context.Context, key string, payload []byte, fireAt time.Time) error
	PopDuePending(ctx context.Context, now time.Time) ([][]byte, error)
	IncrWindowCount(ctx context.Context, key string, window time.Duration) (int64, error)
}
