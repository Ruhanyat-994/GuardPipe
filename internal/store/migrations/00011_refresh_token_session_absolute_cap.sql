-- +goose Up
-- documentation/06-database-design.md §4.3 — BUILD_GUIDE.md Phase 14 session
-- hardening. `created_at` follows this project's normal per-row convention;
-- `family_issued_at` is denormalised (copied unchanged onto every rotated
-- row in a family) so `Refresh()` can enforce an absolute session-lifetime
-- cap with the same single-row-by-hash lookup it already does, instead of a
-- second MIN(created_at) query per refresh. Together with the existing
-- rotate-on-use TTL (the idle timeout), this closes the gap where a
-- continuously-refreshed session — legitimate or a stolen-cookie replay —
-- never expired at all.

ALTER TABLE refresh_tokens
    ADD COLUMN created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN family_issued_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS family_issued_at,
    DROP COLUMN IF EXISTS created_at;
