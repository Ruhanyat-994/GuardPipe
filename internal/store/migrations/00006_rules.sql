-- +goose Up
-- documentation/06-database-design.md §4.15. Rule catalogue, upserted from
-- each engine's code registry at every startup (BUILD_GUIDE.md Phase 5) —
-- this table exists before any engine does, so it starts empty and fills in
-- as codescan/depscan/etc. (Phase 6+) register their RuleMeta.

CREATE TABLE rules (
    id                TEXT PRIMARY KEY,
    engine            engine_id NOT NULL,
    category          TEXT NOT NULL,
    title             TEXT NOT NULL,
    description       TEXT NOT NULL,
    remediation       TEXT NOT NULL,
    default_severity  severity NOT NULL,
    cwe               TEXT[] NOT NULL DEFAULT '{}',
    owasp             TEXT[] NOT NULL DEFAULT '{}',
    "references"      TEXT[] NOT NULL DEFAULT '{}',
    tier              TEXT NOT NULL CHECK (tier IN ('core', 'stretch')),
    -- Lets an operator silence a noisy rule without a deploy.
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rules_engine_tier ON rules (engine, tier);

-- +goose Down
DROP TABLE IF EXISTS rules;
