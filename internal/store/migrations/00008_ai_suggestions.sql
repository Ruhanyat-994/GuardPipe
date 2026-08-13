-- +goose Up
-- documentation/06-database-design.md §4.13. Schema-only, pulled forward
-- from a later phase (Phase 10/11's AI-consuming engines, or Phase 13's
-- explain/patch endpoints — not yet assigned a BUILD_GUIDE.md phase) into
-- Phase 6, for the same reason migration 00005 was pulled into Phase 5:
-- §11's planned migration sequence numbers this 00008, between findings
-- (00007) and dependencies (00009), and shipping around it would leave a
-- gap the migration-numbering test (internal/store/migrate_test.go)
-- correctly rejects. No Go repository exists against this table yet.

CREATE TABLE ai_suggestions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- One suggestion per finding.
    finding_id      UUID NOT NULL UNIQUE REFERENCES findings (id) ON DELETE CASCADE,
    explanation     TEXT,
    patch_diff      TEXT,
    -- 'verified' = dry-run applied cleanly (FR-AI-004).
    patch_status    patch_status NOT NULL DEFAULT 'unverified',
    model           TEXT NOT NULL,
    -- Reproducibility — which prompt produced this.
    prompt_version  TEXT NOT NULL,
    -- Cache key (FR-AI-008).
    input_hash      TEXT NOT NULL,
    tokens_in       INT,
    tokens_out      INT,
    generated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The cache lookup, which is what keeps us inside the free tier.
CREATE INDEX idx_ai_input_hash ON ai_suggestions (input_hash);

-- +goose Down
DROP TABLE IF EXISTS ai_suggestions;
