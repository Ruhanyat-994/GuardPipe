-- +goose Up
-- BUILD_GUIDE.md Phase 14 (Platform admin panel). This is a genuinely new
-- axis, not a stronger version of the existing `user_role` enum
-- (admin/member/viewer, all per-organisation) — a platform operator sees
-- across every tenant, which `user_role` deliberately never allows. See the
-- 2026-08-31 note at the top of BUILD_GUIDE.md's Phase 14 for the full
-- reasoning. This migration also needs `documentation/06-database-design.md`'s
-- normal two-approval schema review before it's "really" done — flagged the
-- same way migration 00011's session-hardening columns were.

-- Deliberately a separate table, not a fourth `user_role` value: platform-
-- operator status is orthogonal to org-scoped role (a GuardPipe team member
-- can hold this while remaining an ordinary `member` of their own org day to
-- day). Provisioned exclusively via `guardpipe admin grant-operator` /
-- `revoke-operator` (cmd/guardpipe/admin.go) — never through an HTTP
-- endpoint, so a compromised operator session can never mint a second
-- operator through the API.
CREATE TABLE platform_operators (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    granted_by UUID REFERENCES users (id) ON DELETE SET NULL,
    note       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Current-state columns only, same "state on the row, history in
-- audit_log" split every other table already uses (findings' own
-- append-only-apart-from-triage rule is the precedent) — every suspend/
-- reinstate call still writes an audit_log row, so the full history
-- survives independently of these columns.
ALTER TABLE organizations ADD COLUMN suspended_at     TIMESTAMPTZ;
ALTER TABLE organizations ADD COLUMN suspended_reason TEXT;
ALTER TABLE users         ADD COLUMN suspended_at     TIMESTAMPTZ;
ALTER TABLE users         ADD COLUMN suspended_reason TEXT;

-- The pentest-misuse intake queue — this is what makes an attributed report
-- of misuse (target_attestations/scans.requested_ip already make it
-- attributable, CLAUDE.md's "Accountability, not an allowlist" section)
-- actionable by a human, not just logged. A `confirmed_misuse` resolution
-- is the trigger for revoking the target (reusing target_status's existing
-- 'revoked' value — no second "blocked" concept).
CREATE TYPE flag_status AS ENUM ('open', 'investigating', 'dismissed', 'confirmed_misuse');

CREATE TABLE pentest_target_flags (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_id    UUID NOT NULL REFERENCES pentest_targets (id) ON DELETE CASCADE,
    status       flag_status NOT NULL DEFAULT 'open',
    source       TEXT NOT NULL CHECK (source IN ('self_reported', 'external_complaint', 'operator_review')),
    reason       TEXT NOT NULL,
    reported_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    resolved_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    resolved_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pentest_target_flags_status ON pentest_target_flags (status, created_at DESC);
CREATE INDEX idx_pentest_target_flags_target ON pentest_target_flags (target_id);

-- +goose Down
DROP TABLE IF EXISTS pentest_target_flags;
DROP TYPE IF EXISTS flag_status;
ALTER TABLE users         DROP COLUMN IF EXISTS suspended_reason;
ALTER TABLE users         DROP COLUMN IF EXISTS suspended_at;
ALTER TABLE organizations DROP COLUMN IF EXISTS suspended_reason;
ALTER TABLE organizations DROP COLUMN IF EXISTS suspended_at;
DROP TABLE IF EXISTS platform_operators;
