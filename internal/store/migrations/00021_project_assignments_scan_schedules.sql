-- +goose Up
-- BUILD_GUIDE.md Phase 15. scan_profile reuses the same {Type, Engines,
-- PentestConfig} shape orchestrator.CreateScanInput already accepts, stored
-- as JSONB the same way scans.pentest_config already is (migration 00014) —
-- one shape for "how to run this scan," not a second one invented for
-- scheduling.

CREATE TABLE project_assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    assigned_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, user_id)
);

CREATE INDEX idx_project_assignments_user ON project_assignments (user_id);
CREATE INDEX idx_project_assignments_project ON project_assignments (project_id);

CREATE TABLE scan_schedules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    cron_expression  TEXT NOT NULL,
    scan_profile     JSONB NOT NULL,
    assigned_to      UUID REFERENCES users (id) ON DELETE SET NULL,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    enabled          BOOLEAN NOT NULL DEFAULT true,
    next_run_at      TIMESTAMPTZ NOT NULL,
    last_run_at      TIMESTAMPTZ,
    last_run_status  TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial index: the scheduler ticker's only query is "what's due, and
-- enabled" — a disabled or far-future row never needs to be scanned.
CREATE INDEX idx_scan_schedules_due ON scan_schedules (next_run_at) WHERE enabled;
CREATE INDEX idx_scan_schedules_project ON scan_schedules (project_id);

-- +goose Down
DROP TABLE IF EXISTS scan_schedules;
DROP TABLE IF EXISTS project_assignments;
