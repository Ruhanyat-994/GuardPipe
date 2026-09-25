package billing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Mode is GUARDPIPE_BILLING_MODE.
type Mode string

const (
	// ModeDemo charges tokens for real and takes "payments" through the
	// demo checkout, which never touches money or card data.
	ModeDemo Mode = "demo"
	// ModeStripe is reserved for a real payment provider (not built).
	ModeStripe Mode = "stripe"
	// ModeOff disables billing: nothing is charged, nothing is gated.
	ModeOff Mode = "off"
)

// Subscription status values (billing_subscriptions.status).
const (
	StatusActive   = "active"
	StatusCanceled = "canceled"
	StatusExpired  = "expired"
)

// Bucket sources.
const (
	SourcePlanGrant  = "plan_grant"
	SourceTopup      = "topup"
	SourceAdjustment = "adjustment"
)

// Ledger kinds.
const (
	KindGrant      = "grant"
	KindTopup      = "topup"
	KindDebit      = "debit"
	KindRefund     = "refund"
	KindExpire     = "expire"
	KindAdjustment = "adjustment"
)

// Checkout statuses.
const (
	CheckoutPending = "pending"
	CheckoutPaid    = "paid"
	CheckoutFailed  = "failed"
	CheckoutExpired = "expired"
)

type Subscription struct {
	OrgID             uuid.UUID
	PlanCode          PlanCode
	Status            string
	PeriodStart       time.Time
	PeriodEnd         time.Time
	NextGrantAt       time.Time
	CancelAtPeriodEnd bool
}

// Bucket is one pot of tokens: a monthly plan grant, a purchased top-up,
// or an operator adjustment.
type Bucket struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	Source     string
	Amount     int64
	Remaining  int64
	ExpiresAt  time.Time
	CheckoutID *uuid.UUID
	CreatedAt  time.Time
}

// LedgerEntry is one append-only billing_ledger row.
type LedgerEntry struct {
	ID             uuid.UUID
	OrgID          uuid.UUID
	Kind           string
	Delta          int64
	BalanceAfter   int64
	BucketID       *uuid.UUID
	ScanID         *uuid.UUID
	ScanJobID      *uuid.UUID
	Engine         *domain.EngineID
	TriggerSource  string
	Reason         string
	PriceVersion   string
	ActorUserID    *uuid.UUID
	IdempotencyKey string
	CreatedAt      time.Time
}

type CheckoutSession struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	CreatedBy   *uuid.UUID
	ItemCode    string
	AmountCents int
	Currency    string
	Provider    string
	ProviderRef string
	Status      string
	PaidAt      *time.Time
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// UsageRow is net tokens spent (debits minus refunds) per engine and
// trigger since a point in time.
type UsageRow struct {
	Engine        domain.EngineID
	TriggerSource string
	Tokens        int64
}

// Repository is the persistence port; the implementation is
// internal/store/repo.BillingRepo. It holds no business rules.
type Repository interface {
	// WithOrgLock runs fn in one transaction that holds a row lock on the
	// org's subscription, so concurrent charges for one org are
	// serialised. If the org has no subscription yet, fresh is inserted
	// first and created is true (the caller then grants its first tokens).
	WithOrgLock(ctx context.Context, orgID uuid.UUID, fresh Subscription, fn func(tx Tx, sub *Subscription, created bool) error) error
	// OrgForJob returns the org that was charged for a scan job, or
	// ok=false if nothing was ever charged for it.
	OrgForJob(ctx context.Context, jobID uuid.UUID) (orgID uuid.UUID, ok bool, err error)
	// DueOrgs lists orgs whose next grant or period end is due.
	DueOrgs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
}

// Tx is everything that runs inside WithOrgLock. Every method is scoped
// to the locked org.
type Tx interface {
	UpdateSubscription(ctx context.Context, sub *Subscription) error
	// SpendableBuckets: remaining > 0 and not expired, soonest-expiring first.
	SpendableBuckets(ctx context.Context, now time.Time) ([]Bucket, error)
	// BucketsBySource: remaining > 0, regardless of expiry.
	BucketsBySource(ctx context.Context, source string) ([]Bucket, error)
	// ExpiredBuckets: remaining > 0 and expires_at <= now.
	ExpiredBuckets(ctx context.Context, now time.Time) ([]Bucket, error)
	GetBucket(ctx context.Context, id uuid.UUID) (*Bucket, error)
	InsertBucket(ctx context.Context, b *Bucket) error
	// AddToBucket changes remaining by delta (negative spends).
	AddToBucket(ctx context.Context, id uuid.UUID, delta int64) error
	// InsertLedger returns inserted=false if the idempotency key already
	// exists — the caller must then skip the matching bucket change.
	InsertLedger(ctx context.Context, e *LedgerEntry) (inserted bool, err error)
	LedgerKeyExists(ctx context.Context, key string) (bool, error)
	DebitsForJob(ctx context.Context, jobID uuid.UUID) ([]LedgerEntry, error)
	Balance(ctx context.Context, now time.Time) (int64, error)
	ListLedger(ctx context.Context, before *time.Time, limit int) ([]LedgerEntry, error)
	LedgerForScan(ctx context.Context, scanID uuid.UUID) ([]LedgerEntry, error)
	UsageSince(ctx context.Context, since time.Time) ([]UsageRow, error)
	InsertCheckout(ctx context.Context, c *CheckoutSession) error
	// GetCheckout returns a not-found error for another org's session.
	GetCheckout(ctx context.Context, id uuid.UUID) (*CheckoutSession, error)
	SetCheckoutStatus(ctx context.Context, id uuid.UUID, status string, paidAt *time.Time) error
}

// PaymentProvider is the seam a real provider (Stripe) plugs into later.
type PaymentProvider interface {
	Name() string
	// CreateSession returns where to send the buyer, plus the provider's
	// own reference for the session.
	CreateSession(ctx context.Context, s CheckoutSession) (redirectURL, providerRef string, err error)
}

// ChargeLine is one scan job to be charged.
type ChargeLine struct {
	JobID  uuid.UUID
	Engine domain.EngineID
}

// ChargeRequest is everything Charge needs to price and record a scan.
// The price itself is computed here, from the catalogue — never passed in.
type ChargeRequest struct {
	ScanID  uuid.UUID
	Lines   []ChargeLine
	Preset  domain.PentestPreset
	Trigger domain.TriggerSource
	ActorID uuid.UUID
}

// Summary is GET /billing/summary.
type Summary struct {
	Plan               Plan
	Status             string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	NextGrantAt        time.Time
	CancelAtPeriodEnd  bool
	Balance            int64
	PlanGrantRemaining int64
	TopupRemaining     int64
	TopupExpiresAt     *time.Time
	UsedThisCycle      int64
	Usage              []UsageRow
	Mode               Mode
}

// Estimate is POST /billing/estimate.
type Estimate struct {
	Quote        Quote
	Balance      int64
	BalanceAfter int64
	Affordable   bool
	PlanAllowed  bool
	// BlockedEngines are the requested engines the current plan can't run.
	BlockedEngines []domain.EngineID
	RequiredPlan   PlanCode
}

// ScanTokens is what one scan cost.
type ScanTokens struct {
	Charged  int64
	Refunded int64
	Entries  []LedgerEntry
}
