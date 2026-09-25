-- +goose Up
-- BUILD_GUIDE.md Phase 17 Part B — GitHub webhook live scanning.
--
-- project_webhooks: one row per project with live scanning turned on. The
-- row existing IS "enabled" (turning it off deletes the row and the hook on
-- GitHub) — no separate enabled flag to drift out of sync with GitHub.
-- engines is the user's own explicit choice made when turning it on (never
-- a replay of whatever the last manual scan happened to run), and can never
-- contain pentest: that's enforced by the service layer AND by the CHECK
-- below, so no code path or hand-edited row can make a push start a live
-- penetration test.
CREATE TABLE project_webhooks (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id            UUID NOT NULL UNIQUE REFERENCES projects (id) ON DELETE CASCADE,
    github_hook_id        BIGINT NOT NULL,
    secret_ciphertext     BYTEA NOT NULL,
    secret_nonce          BYTEA NOT NULL,
    engines               engine_id[] NOT NULL,
    watched_branches      TEXT[] NOT NULL,
    -- enabled_by + attested_at are the "testimony": who authorised automatic
    -- scanning of this repository, and when they ticked the confirmation.
    -- Every webhook-triggered scan is attributed to enabled_by.
    enabled_by            UUID REFERENCES users (id) ON DELETE SET NULL,
    attested_at           TIMESTAMPTZ NOT NULL,
    last_delivery_at      TIMESTAMPTZ,
    last_delivery_status  TEXT,
    -- Set by the circuit breaker; cleared when a user re-confirms. NULL means
    -- live scanning is active.
    paused_reason         TEXT,
    paused_at             TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT project_webhooks_no_pentest CHECK (NOT ('pentest' = ANY (engines))),
    CONSTRAINT project_webhooks_engines_nonempty CHECK (cardinality(engines) > 0),
    CONSTRAINT project_webhooks_branches_nonempty CHECK (cardinality(watched_branches) > 0)
);

-- Where a scan came from. Nullable: every scan created before this migration
-- simply has no recorded origin. TEXT + CHECK rather than a native enum —
-- this set is expected to grow (documentation/06-database-design.md's
-- enum-vs-CHECK convention). cli_watch is reserved for the later CLI git
-- hook so that feature doesn't need its own migration.
ALTER TABLE scans
    ADD COLUMN trigger_source TEXT
        CHECK (trigger_source IN ('manual', 'scheduled', 'webhook_push', 'webhook_pull_request', 'cli_watch')),
    ADD COLUMN trigger_ref    TEXT,
    ADD COLUMN trigger_actor  TEXT;

-- +goose Down
ALTER TABLE scans
    DROP COLUMN IF EXISTS trigger_actor,
    DROP COLUMN IF EXISTS trigger_ref,
    DROP COLUMN IF EXISTS trigger_source;
DROP TABLE IF EXISTS project_webhooks;
