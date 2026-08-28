-- +goose Up
-- documentation/06-database-design.md §4.14/§4.17. Both tables were
-- deliberately deferred to Phase 13 by the migrations that own their
-- siblings: 00007_findings.sql (finding_status_history, "once triage
-- exists to write history for") and 00009_dependencies_scan_evidence.sql
-- (risk_assessments, "Phase 13 (scoring) owns that table").

-- Append-only triage audit (DR-003's mutable-field-group rule on findings
-- gets its paper trail here — every status change on a finding writes one
-- row, never updated or deleted).
CREATE TABLE finding_status_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id   UUID NOT NULL REFERENCES findings (id) ON DELETE CASCADE,
    from_status  finding_status NOT NULL,
    to_status    finding_status NOT NULL,
    reason       TEXT,
    changed_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_finding_status_history_finding ON finding_status_history (finding_id, changed_at);

-- One row per scan, written once scoring runs. engine_scores/breakdown are
-- JSONB because their shape is genuinely variable (a per-engine map, an
-- ordered list of differently-shaped contribution reasons) and neither is
-- ever filtered or joined on — the one case CLAUDE.md's JSONB convention
-- allows.
CREATE TABLE risk_assessments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id           UUID NOT NULL UNIQUE REFERENCES scans (id) ON DELETE CASCADE,
    score             INT NOT NULL CHECK (score >= 0 AND score <= 100),
    verdict           verdict NOT NULL,
    -- {"codescan": 62, "k8sscan": 40, ...}
    engine_scores     JSONB NOT NULL DEFAULT '{}',
    -- Ordered contributions the UI renders under the gauge.
    breakdown         JSONB NOT NULL DEFAULT '[]',
    -- Score of the most recent prior completed scan of the same project,
    -- for the delta (FR-SCR-008). NULL when there is no previous scan.
    previous_score    INT CHECK (previous_score IS NULL OR (previous_score >= 0 AND previous_score <= 100)),
    is_partial        BOOLEAN NOT NULL DEFAULT false,
    -- So historical scores stay interpretable when the formula changes
    -- (documentation/11-risk-scoring-and-severity.md §7). Never bump an
    -- existing row's version — recompute as a new one instead.
    formula_version   TEXT NOT NULL,
    computed_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS risk_assessments;
DROP TABLE IF EXISTS finding_status_history;
