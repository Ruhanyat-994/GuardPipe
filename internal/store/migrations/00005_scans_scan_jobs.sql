-- +goose Up
-- documentation/06-database-design.md §4.9-4.10. Schema-only, pulled forward
-- from Phase 6 (`modules/orchestrator`) into Phase 5: §11's planned
-- migration sequence numbers this 00005, one below rules' 00006, so
-- creating 00006 first would leave a gap the migration-numbering test
-- (internal/store/migrate_test.go) correctly rejects. No Go repository
-- exists against this table yet — that lands with the orchestrator itself.

CREATE TABLE scans (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    triggered_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    type               scan_type NOT NULL,
    status             scan_status NOT NULL DEFAULT 'queued',
    -- What the user asked for.
    requested_engines  engine_id[] NOT NULL,
    -- Resolved at clone; makes the scan reproducible.
    commit_sha         TEXT,
    branch             TEXT,
    -- Polled by workers (FR-ORC-008).
    cancel_requested   BOOLEAN NOT NULL DEFAULT false,
    -- Only for whole-scan failure.
    error_reason       TEXT,
    queued_at          TIMESTAMPTZ,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    -- Denormalised {critical:n, high:n, ...} for fast list rendering,
    -- written once in the same transaction that finalises the scan.
    finding_counts     JSONB NOT NULL DEFAULT '{}',
    -- Not in §4.9's column table as printed, but idx_scans_project_created
    -- (also §4.9) orders by created_at, so the column has to exist —
    -- same class of gap project_credentials' migration comment already
    -- flagged once for §4.6. updated_at added alongside it for the same
    -- universal convention every other table follows.
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scans_project_created ON scans (project_id, created_at DESC);
CREATE INDEX idx_scans_status ON scans (status) WHERE status IN ('queued', 'running');

CREATE TABLE scan_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id       UUID NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    engine        engine_id NOT NULL,
    status        job_status NOT NULL DEFAULT 'queued',
    attempt       INT NOT NULL DEFAULT 0,
    -- NULL until claimed; the reaper compares against this.
    claimed_at    TIMESTAMPTZ,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    -- e.g. "timeout", "panic", "ai_unavailable".
    error_reason  TEXT,
    -- e.g. "no_target_artifacts", "docker_unavailable".
    skip_reason   TEXT,
    -- files_scanned, rules_evaluated, duration_ms.
    stats         JSONB NOT NULL DEFAULT '{}',
    UNIQUE (scan_id, engine)
);

CREATE INDEX idx_jobs_status_claimed ON scan_jobs (status, claimed_at) WHERE status = 'running';

-- +goose Down
DROP TABLE IF EXISTS scan_jobs;
DROP TABLE IF EXISTS scans;
