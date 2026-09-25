package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/tx"
)

// BillingRepo implements billing.Repository against the billing_* tables
// (migration 00028). SQL only — every rule lives in modules/billing.
type BillingRepo struct {
	db interface {
		Querier
		tx.Beginner
	}
}

func NewBillingRepo(db interface {
	Querier
	tx.Beginner
}) *BillingRepo {
	return &BillingRepo{db: db}
}

var _ billing.Repository = (*BillingRepo)(nil)

func (r *BillingRepo) WithOrgLock(ctx context.Context, orgID uuid.UUID, fresh billing.Subscription, fn func(billing.Tx, *billing.Subscription, bool) error) error {
	return tx.WithTx(ctx, r.db, func(t pgx.Tx) error {
		// Insert-if-missing first: a concurrent caller waits here on the
		// primary key until the first one commits, then does nothing.
		var created bool
		err := t.QueryRow(ctx, `
			INSERT INTO billing_subscriptions (org_id, plan_code, status, current_period_start, current_period_end, next_grant_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (org_id) DO NOTHING
			RETURNING true`,
			orgID, string(fresh.PlanCode), fresh.Status, fresh.PeriodStart, fresh.PeriodEnd, fresh.NextGrantAt,
		).Scan(&created)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("repo: provision subscription: %w", err)
		}
		var sub billing.Subscription
		var plan string
		err = t.QueryRow(ctx, `
			SELECT org_id, plan_code, status, current_period_start, current_period_end, next_grant_at, cancel_at_period_end
			FROM billing_subscriptions WHERE org_id = $1 FOR UPDATE`, orgID,
		).Scan(&sub.OrgID, &plan, &sub.Status, &sub.PeriodStart, &sub.PeriodEnd, &sub.NextGrantAt, &sub.CancelAtPeriodEnd)
		if err != nil {
			return fmt.Errorf("repo: lock subscription: %w", err)
		}
		sub.PlanCode = billing.PlanCode(plan)
		return fn(&billingTx{t: t, orgID: orgID}, &sub, created)
	})
}

func (r *BillingRepo) OrgForJob(ctx context.Context, jobID uuid.UUID) (uuid.UUID, bool, error) {
	var orgID uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT org_id FROM billing_ledger WHERE scan_job_id = $1 AND kind = 'debit' LIMIT 1`, jobID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("repo: org for job: %w", err)
	}
	return orgID, true, nil
}

func (r *BillingRepo) DueOrgs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT org_id FROM billing_subscriptions WHERE next_grant_at <= $1
		UNION
		SELECT DISTINCT org_id FROM billing_token_buckets WHERE remaining > 0 AND expires_at <= $1
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("repo: due orgs: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// billingTx is billing.Tx bound to one transaction and one locked org.
type billingTx struct {
	t     pgx.Tx
	orgID uuid.UUID
}

func (b *billingTx) UpdateSubscription(ctx context.Context, s *billing.Subscription) error {
	_, err := b.t.Exec(ctx, `
		UPDATE billing_subscriptions
		SET plan_code = $2, status = $3, current_period_start = $4, current_period_end = $5,
		    next_grant_at = $6, cancel_at_period_end = $7, updated_at = now()
		WHERE org_id = $1`,
		b.orgID, string(s.PlanCode), s.Status, s.PeriodStart, s.PeriodEnd, s.NextGrantAt, s.CancelAtPeriodEnd)
	if err != nil {
		return fmt.Errorf("repo: update subscription: %w", err)
	}
	return nil
}

const bucketColumns = `SELECT id, org_id, source, amount, remaining, expires_at, checkout_id, created_at FROM billing_token_buckets`

func (b *billingTx) buckets(ctx context.Context, where string, args ...any) ([]billing.Bucket, error) {
	rows, err := b.t.Query(ctx, bucketColumns+" WHERE org_id = $1 AND "+where+" ORDER BY expires_at, created_at", append([]any{b.orgID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("repo: list buckets: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (billing.Bucket, error) {
		var bk billing.Bucket
		err := row.Scan(&bk.ID, &bk.OrgID, &bk.Source, &bk.Amount, &bk.Remaining, &bk.ExpiresAt, &bk.CheckoutID, &bk.CreatedAt)
		return bk, err
	})
}

func (b *billingTx) SpendableBuckets(ctx context.Context, now time.Time) ([]billing.Bucket, error) {
	return b.buckets(ctx, "remaining > 0 AND expires_at > $2", now)
}

func (b *billingTx) BucketsBySource(ctx context.Context, source string) ([]billing.Bucket, error) {
	return b.buckets(ctx, "remaining > 0 AND source = $2", source)
}

func (b *billingTx) ExpiredBuckets(ctx context.Context, now time.Time) ([]billing.Bucket, error) {
	return b.buckets(ctx, "remaining > 0 AND expires_at <= $2", now)
}

func (b *billingTx) GetBucket(ctx context.Context, id uuid.UUID) (*billing.Bucket, error) {
	list, err := b.buckets(ctx, "id = $2", id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, apperrors.NotFound("billing.bucket_not_found", "bucket not found")
	}
	return &list[0], nil
}

func (b *billingTx) InsertBucket(ctx context.Context, bk *billing.Bucket) error {
	err := b.t.QueryRow(ctx, `
		INSERT INTO billing_token_buckets (id, org_id, source, amount, remaining, expires_at, checkout_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING created_at`,
		bk.ID, b.orgID, bk.Source, bk.Amount, bk.Remaining, bk.ExpiresAt, bk.CheckoutID,
	).Scan(&bk.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert bucket: %w", err)
	}
	return nil
}

func (b *billingTx) AddToBucket(ctx context.Context, id uuid.UUID, delta int64) error {
	tag, err := b.t.Exec(ctx, `UPDATE billing_token_buckets SET remaining = remaining + $3, updated_at = now() WHERE id = $1 AND org_id = $2`, id, b.orgID, delta)
	if err != nil {
		return fmt.Errorf("repo: update bucket: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("repo: update bucket: bucket %s not found", id)
	}
	return nil
}

// InsertLedger stamps created_at with clock_timestamp(), not now(): now()
// is the transaction's start time, so every row one charge writes would
// share a timestamp — breaking both their order and the ledger's
// created_at pagination cursor.
func (b *billingTx) InsertLedger(ctx context.Context, e *billing.LedgerEntry) (bool, error) {
	var engine *string
	if e.Engine != nil {
		s := string(*e.Engine)
		engine = &s
	}
	err := b.t.QueryRow(ctx, `
		INSERT INTO billing_ledger (id, org_id, kind, delta, balance_after, bucket_id, scan_id, scan_job_id, engine,
			trigger_source, reason, price_version, actor_user_id, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::engine_id, NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), $13, $14,
			clock_timestamp())
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING created_at`,
		e.ID, b.orgID, e.Kind, e.Delta, e.BalanceAfter, e.BucketID, e.ScanID, e.ScanJobID, engine,
		e.TriggerSource, e.Reason, e.PriceVersion, e.ActorUserID, e.IdempotencyKey,
	).Scan(&e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repo: insert ledger: %w", err)
	}
	return true, nil
}

func (b *billingTx) LedgerKeyExists(ctx context.Context, key string) (bool, error) {
	var exists bool
	if err := b.t.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing_ledger WHERE idempotency_key = $1)`, key).Scan(&exists); err != nil {
		return false, fmt.Errorf("repo: ledger key: %w", err)
	}
	return exists, nil
}

const ledgerColumns = `
	SELECT id, org_id, kind, delta, balance_after, bucket_id, scan_id, scan_job_id, engine::text,
	       COALESCE(trigger_source, ''), COALESCE(reason, ''), COALESCE(price_version, ''), actor_user_id,
	       idempotency_key, created_at
	FROM billing_ledger`

func (b *billingTx) ledger(ctx context.Context, where, tail string, args ...any) ([]billing.LedgerEntry, error) {
	rows, err := b.t.Query(ctx, ledgerColumns+" WHERE org_id = $1 AND "+where+" "+tail, append([]any{b.orgID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("repo: list ledger: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (billing.LedgerEntry, error) {
		var e billing.LedgerEntry
		var engine *string
		err := row.Scan(&e.ID, &e.OrgID, &e.Kind, &e.Delta, &e.BalanceAfter, &e.BucketID, &e.ScanID, &e.ScanJobID, &engine,
			&e.TriggerSource, &e.Reason, &e.PriceVersion, &e.ActorUserID, &e.IdempotencyKey, &e.CreatedAt)
		if engine != nil {
			id := domain.EngineID(*engine)
			e.Engine = &id
		}
		return e, err
	})
}

func (b *billingTx) DebitsForJob(ctx context.Context, jobID uuid.UUID) ([]billing.LedgerEntry, error) {
	return b.ledger(ctx, "scan_job_id = $2 AND kind = 'debit'", "ORDER BY created_at", jobID)
}

func (b *billingTx) Balance(ctx context.Context, now time.Time) (int64, error) {
	var bal int64
	err := b.t.QueryRow(ctx, `SELECT COALESCE(SUM(remaining), 0) FROM billing_token_buckets WHERE org_id = $1 AND remaining > 0 AND expires_at > $2`, b.orgID, now).Scan(&bal)
	if err != nil {
		return 0, fmt.Errorf("repo: balance: %w", err)
	}
	return bal, nil
}

func (b *billingTx) ListLedger(ctx context.Context, before *time.Time, limit int) ([]billing.LedgerEntry, error) {
	if before != nil {
		return b.ledger(ctx, "created_at < $2", "ORDER BY created_at DESC, id DESC LIMIT $3", *before, limit)
	}
	return b.ledger(ctx, "true", "ORDER BY created_at DESC, id DESC LIMIT $2", limit)
}

func (b *billingTx) LedgerForScan(ctx context.Context, scanID uuid.UUID) ([]billing.LedgerEntry, error) {
	return b.ledger(ctx, "scan_id = $2", "ORDER BY created_at", scanID)
}

func (b *billingTx) UsageSince(ctx context.Context, since time.Time) ([]billing.UsageRow, error) {
	rows, err := b.t.Query(ctx, `
		SELECT engine::text, COALESCE(trigger_source, 'manual'), -SUM(delta)
		FROM billing_ledger
		WHERE org_id = $1 AND kind IN ('debit', 'refund') AND created_at >= $2 AND engine IS NOT NULL
		GROUP BY engine, COALESCE(trigger_source, 'manual')
		HAVING SUM(delta) <> 0
		ORDER BY 3 DESC`, b.orgID, since)
	if err != nil {
		return nil, fmt.Errorf("repo: usage: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (billing.UsageRow, error) {
		var u billing.UsageRow
		var engine string
		err := row.Scan(&engine, &u.TriggerSource, &u.Tokens)
		u.Engine = domain.EngineID(engine)
		return u, err
	})
}

func (b *billingTx) InsertCheckout(ctx context.Context, c *billing.CheckoutSession) error {
	_, err := b.t.Exec(ctx, `
		INSERT INTO billing_checkout_sessions (id, org_id, created_by, item_code, amount_cents, currency, provider,
			provider_ref, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $11)`,
		c.ID, b.orgID, c.CreatedBy, c.ItemCode, c.AmountCents, c.Currency, c.Provider, c.ProviderRef, c.Status, c.ExpiresAt, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert checkout: %w", err)
	}
	return nil
}

func (b *billingTx) GetCheckout(ctx context.Context, id uuid.UUID) (*billing.CheckoutSession, error) {
	var c billing.CheckoutSession
	var ref *string
	err := b.t.QueryRow(ctx, `
		SELECT id, org_id, created_by, item_code, amount_cents, currency, provider, provider_ref, status, paid_at, expires_at, created_at
		FROM billing_checkout_sessions WHERE id = $1 AND org_id = $2`, id, b.orgID,
	).Scan(&c.ID, &c.OrgID, &c.CreatedBy, &c.ItemCode, &c.AmountCents, &c.Currency, &c.Provider, &ref, &c.Status, &c.PaidAt, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("billing.checkout_not_found", "checkout not found")
	}
	if err != nil {
		return nil, fmt.Errorf("repo: get checkout: %w", err)
	}
	if ref != nil {
		c.ProviderRef = *ref
	}
	return &c, nil
}

func (b *billingTx) SetCheckoutStatus(ctx context.Context, id uuid.UUID, status string, paidAt *time.Time) error {
	_, err := b.t.Exec(ctx, `UPDATE billing_checkout_sessions SET status = $3, paid_at = COALESCE($4, paid_at), updated_at = now() WHERE id = $1 AND org_id = $2`,
		id, b.orgID, status, paidAt)
	if err != nil {
		return fmt.Errorf("repo: update checkout: %w", err)
	}
	return nil
}
