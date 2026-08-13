-- +goose Up
-- documentation/06-database-design.md §4.19. Append-only — no UPDATE, no
-- DELETE, enforced by only ever exposing an insert method (DR-007).

CREATE TABLE audit_log (
    id             BIGSERIAL PRIMARY KEY, -- sequential — ordering is the point here
    org_id         UUID,
    -- Null for system actions.
    actor_id       UUID,
    -- e.g. "auth.login", "scan.started", "finding.suppressed", "target.attested".
    action         TEXT NOT NULL,
    resource_type  TEXT,
    resource_id    UUID,
    detail         JSONB NOT NULL DEFAULT '{}',
    ip             INET,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_org_created ON audit_log (org_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
