-- +goose Up
-- documentation/06-database-design.md's `documents` table (Phase 11,
-- docreview) — a project-scoped uploaded document, reviewed by the
-- docreview engine alongside anything discovered inside the repository
-- checkout itself. Content is stored directly as BYTEA rather than in
-- object storage: the module spec's own cap (20 documents x 100 KB, now per
-- project rather than per scan — documents are uploaded once and reused by
-- every scan) tops out at 2 MB per project, well within "just a column,"
-- and avoids a new dependency for a zero-budget project.

CREATE TABLE documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    -- SET NULL, not RESTRICT: unlike a pentest attestation (a legal record
    -- that must always name who accepted it), an uploaded document has no
    -- such requirement — a deleted user's past uploads should still be
    -- reviewable, just no longer attributable to a specific account.
    uploaded_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    filename     TEXT NOT NULL,
    mime_type    TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    content      BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_documents_project ON documents (project_id);

-- +goose Down
DROP TABLE IF EXISTS documents;
