-- +goose Up
-- documentation/06-database-design.md §4.5. A stored credential going bad
-- (expired/revoked PAT) must never be silently indistinguishable from
-- "everything's fine" — the project and repository row stay exactly where
-- they are (no auto-delete/auto-archive on a failed scan), but there was
-- previously no persisted signal at all: the only way to learn a credential
-- was bad was a scan failing with a generic "workspace_unavailable" reason,
-- with nothing surfaced on the project itself between scans. NULL means
-- healthy (or never checked); a non-null credential_invalid_at means the
-- orchestrator hit a 401/403 cloning with this credential and the project
-- needs the user to reattach one. Cleared back to NULL by
-- RepositoryRepository.Upsert — i.e. the existing attach/replace flow — the
-- next time a credential is (re)attached.

ALTER TABLE repositories
    ADD COLUMN credential_invalid_at     TIMESTAMPTZ,
    ADD COLUMN credential_invalid_reason TEXT;

-- +goose Down
ALTER TABLE repositories
    DROP COLUMN IF EXISTS credential_invalid_reason,
    DROP COLUMN IF EXISTS credential_invalid_at;
