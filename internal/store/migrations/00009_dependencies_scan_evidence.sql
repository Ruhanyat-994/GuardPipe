-- +goose Up
-- documentation/06-database-design.md §4.16/§4.18. risk_assessments (§4.17)
-- is excluded — BUILD_GUIDE.md Phase 6 scopes this migration to the
-- dependency/evidence half only; Phase 13 (scoring) owns that table.

-- The SBOM-adjacent inventory from depscan. Separate from findings because
-- a dependency is an asset, not an issue.
CREATE TABLE dependencies (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id        UUID NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    ecosystem      TEXT NOT NULL CHECK (ecosystem IN ('npm', 'pypi', 'go', 'maven', 'composer')),
    name           TEXT NOT NULL,
    version        TEXT NOT NULL,
    is_direct      BOOLEAN NOT NULL,
    manifest_path  TEXT NOT NULL,
    declared_range TEXT,
    license        TEXT,
    UNIQUE (scan_id, ecosystem, name, version, manifest_path)
);

-- "Which projects use this package."
CREATE INDEX idx_deps_lookup ON dependencies (ecosystem, name, version);

-- Scan-level artifacts — chiefly the pentest command transcript
-- (FR-PEN-011); depscan doesn't use this table yet (that's Phase 12), the
-- table is created now purely because it shares this migration slot with
-- dependencies per §11's planned sequence.
CREATE TABLE scan_evidence (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id     UUID NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('pentest_transcript', 'sbom', 'image_manifest')),
    -- Redacted before storage, same rule as finding_evidence.content.
    content     TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS scan_evidence;
DROP TABLE IF EXISTS dependencies;
