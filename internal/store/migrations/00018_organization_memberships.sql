-- +goose Up
-- BUILD_GUIDE.md Phase 15 (Organization identity). `users.org_id` (a user's
-- own personal/home organisation, created at registration) is left
-- completely unchanged by this migration — this table is purely additive:
-- it lets a user *also* hold a role in one or more OTHER organisations they
-- were invited into, on top of owning their own. This is what makes the
-- existing per-*user* `role` column effectively per-*membership* without
-- touching the org-isolation guarantee the 2026-08-01 hotfix protects (see
-- CLAUDE.md's "Multi-tenancy" section). This migration needs
-- documentation/06-database-design.md's normal two-approval schema review
-- before it's "really" done — same doc-debt flag migration 00017 carried.

CREATE TABLE organization_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role        user_role NOT NULL DEFAULT 'member',
    invited_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id)
);

CREATE INDEX idx_org_memberships_user ON organization_memberships (user_id);
CREATE INDEX idx_org_memberships_org ON organization_memberships (org_id);

-- +goose Down
DROP TABLE IF EXISTS organization_memberships;
