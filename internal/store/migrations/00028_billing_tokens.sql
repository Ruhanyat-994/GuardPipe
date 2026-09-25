-- +goose Up
-- Token-based subscription billing (TOKENIZATION-ARCHITECTURE.md; supersedes
-- BUILD_GUIDE.md Phase 17 Part A's pentest-only sketch).
--
-- Every organisation has one token balance, made of buckets: the monthly
-- plan grant (expires at the end of its month) and purchased top-up packs
-- (expire after 12 months). Scans spend from the soonest-expiring bucket
-- first. billing_ledger is the append-only record of every change — the
-- token bar, the usage history and any "where did my tokens go?" question
-- are all answered from it.
--
-- Prices, plans and packs live in Go code (internal/modules/billing/
-- catalog.go), not here — these tables only store codes and amounts.

-- One checkout attempt. provider='demo' until a real payment provider
-- exists; a real provider would fill provider_ref.
CREATE TABLE billing_checkout_sessions (
    id            UUID PRIMARY KEY,
    org_id        UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    item_code     TEXT NOT NULL,
    amount_cents  INTEGER NOT NULL CHECK (amount_cents >= 0),
    currency      TEXT NOT NULL DEFAULT 'usd',
    provider      TEXT NOT NULL CHECK (provider IN ('demo', 'stripe')),
    provider_ref  TEXT,
    status        TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'failed', 'expired')),
    paid_at       TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_checkout_sessions_org ON billing_checkout_sessions (org_id, created_at DESC);

-- One row per organisation, created lazily the first time billing looks at
-- the org (a free subscription). Locking this row (SELECT ... FOR UPDATE)
-- is what serialises concurrent charges for one org.
CREATE TABLE billing_subscriptions (
    org_id               UUID PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    plan_code            TEXT NOT NULL CHECK (plan_code IN ('free', 'pro_monthly', 'pro_annual')),
    status               TEXT NOT NULL CHECK (status IN ('active', 'canceled', 'expired', 'past_due')),
    current_period_start TIMESTAMPTZ NOT NULL,
    -- End of what was paid for (1 month or 1 year). The same plan can't be
    -- bought again before this.
    current_period_end   TIMESTAMPTZ NOT NULL,
    -- When the next monthly token grant is due — monthly even on annual.
    next_grant_at        TIMESTAMPTZ NOT NULL,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_subscriptions_due ON billing_subscriptions (next_grant_at);

CREATE TABLE billing_token_buckets (
    id           UUID PRIMARY KEY,
    org_id       UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('plan_grant', 'topup', 'adjustment')),
    amount       BIGINT NOT NULL CHECK (amount > 0),
    -- The database itself refuses a negative balance, whatever the code does.
    remaining    BIGINT NOT NULL CHECK (remaining >= 0 AND remaining <= amount),
    expires_at   TIMESTAMPTZ NOT NULL,
    checkout_id  UUID REFERENCES billing_checkout_sessions (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_billing_token_buckets_spendable ON billing_token_buckets (org_id, expires_at) WHERE remaining > 0;

-- Append-only, like findings: rows are inserted, never updated or deleted.
CREATE TABLE billing_ledger (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('grant', 'topup', 'debit', 'refund', 'expire', 'adjustment')),
    delta           BIGINT NOT NULL,
    balance_after   BIGINT NOT NULL,
    bucket_id       UUID REFERENCES billing_token_buckets (id) ON DELETE SET NULL,
    -- Soft references, deliberately without foreign keys: tokens are
    -- charged before the scan row exists, and the ledger must outlive a
    -- deleted scan.
    scan_id         UUID,
    scan_job_id     UUID,
    engine          engine_id,
    trigger_source  TEXT,
    reason          TEXT,
    price_version   TEXT,
    actor_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    -- Makes every charge, refund and grant safe to retry: the same key can
    -- only ever be written once.
    idempotency_key TEXT NOT NULL UNIQUE,
    -- clock_timestamp(), not now(): rows written by one transaction must
    -- still be ordered, and created_at is the history's pagination cursor.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_billing_ledger_org_created ON billing_ledger (org_id, created_at DESC);
CREATE INDEX idx_billing_ledger_scan ON billing_ledger (scan_id) WHERE scan_id IS NOT NULL;
CREATE INDEX idx_billing_ledger_job ON billing_ledger (scan_job_id) WHERE scan_job_id IS NOT NULL;

-- Live scans stop once the balance would fall below this percentage of the
-- plan's monthly grant, so automation can't use up the tokens a person
-- needs for a manual scan.
ALTER TABLE project_webhooks
    ADD COLUMN live_scan_min_balance_percent SMALLINT NOT NULL DEFAULT 10
        CHECK (live_scan_min_balance_percent BETWEEN 0 AND 90);

-- +goose Down
ALTER TABLE project_webhooks DROP COLUMN IF EXISTS live_scan_min_balance_percent;
DROP TABLE IF EXISTS billing_ledger;
DROP TABLE IF EXISTS billing_token_buckets;
DROP TABLE IF EXISTS billing_subscriptions;
DROP TABLE IF EXISTS billing_checkout_sessions;
