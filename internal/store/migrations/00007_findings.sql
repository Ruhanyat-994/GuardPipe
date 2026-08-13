-- +goose Up
-- documentation/06-database-design.md §4.11-4.12. finding_status_history is
-- explicitly excluded (Phase 13's, once triage exists to write history
-- for) — BUILD_GUIDE.md Phase 6 scopes this migration to the findings half
-- only.

CREATE TABLE findings (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id            UUID NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    job_id             UUID NOT NULL REFERENCES scan_jobs (id) ON DELETE CASCADE,
    -- Denormalised — enables cross-scan queries without a join.
    project_id         UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    engine             engine_id NOT NULL,
    rule_id            TEXT NOT NULL REFERENCES rules (id) ON DELETE RESTRICT,
    source             finding_source NOT NULL DEFAULT 'rule',
    -- SHA-256 hex; cross-scan identity (DR-006).
    fingerprint        TEXT NOT NULL,
    title              TEXT NOT NULL,
    description        TEXT NOT NULL,
    severity           severity NOT NULL,
    confidence         confidence NOT NULL DEFAULT 'medium',
    cwe                TEXT[] NOT NULL DEFAULT '{}',
    cve                TEXT[] NOT NULL DEFAULT '{}',
    owasp              TEXT[] NOT NULL DEFAULT '{}',
    cvss_score         NUMERIC(3,1) CHECK (cvss_score IS NULL OR (cvss_score >= 0.0 AND cvss_score <= 10.0)),
    cvss_vector        TEXT,
    -- Discriminated union — documentation/06-database-design.md §5.
    location           JSONB NOT NULL,
    -- Deterministic guidance from the rule; must stand alone without AI.
    remediation        TEXT NOT NULL,
    -- The only mutable field group (DR-003) — application code may only
    -- UPDATE status/status_reason/status_changed_by/status_changed_at/
    -- updated_at. Every other column is write-once, enforced by code
    -- review and by a repository API with no general update method.
    status             finding_status NOT NULL DEFAULT 'open',
    status_reason      TEXT,
    status_changed_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    status_changed_at  TIMESTAMPTZ,
    metadata           JSONB NOT NULL DEFAULT '{}',
    search_vector      TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || description)) STORED,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Makes re-runs idempotent (NFR-REL-002); bulk insert uses
    -- ON CONFLICT (scan_id, fingerprint) DO NOTHING.
    UNIQUE (scan_id, fingerprint)
);

CREATE INDEX idx_findings_scan_severity ON findings (scan_id, severity, created_at);
CREATE INDEX idx_findings_project_fingerprint ON findings (project_id, fingerprint);
CREATE INDEX idx_findings_scan_engine ON findings (scan_id, engine);
CREATE INDEX idx_findings_status ON findings (scan_id, status) WHERE status = 'open';
CREATE INDEX idx_findings_cve ON findings USING GIN (cve);
CREATE INDEX idx_findings_search ON findings USING GIN (search_vector);
CREATE INDEX idx_findings_file_path ON findings ((location ->> 'path')) WHERE location ->> 'type' = 'file';

CREATE TABLE finding_evidence (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id        UUID NOT NULL REFERENCES findings (id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('code_snippet', 'command_output', 'http_response', 'manifest_excerpt', 'layer_reference')),
    -- Secret values redacted before storage (documentation/06-database-design.md
    -- §4.12) — a finding that detects a hardcoded credential stores the
    -- location and shape of the secret, never the secret itself.
    content           TEXT NOT NULL,
    content_redacted  BOOLEAN NOT NULL DEFAULT false,
    line_start        INT,
    line_end          INT,
    ordinal           INT NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS finding_evidence;
DROP TABLE IF EXISTS findings;
