-- +goose Up
-- BUILD_GUIDE.md Phase 15. `POST /auth/switch-org` re-issues a token pair
-- scoped to a non-home organisation the caller is a member of
-- (organization_memberships, migration 00018). Without recording which org
-- a refresh token is scoped to, `identity.Service.Refresh` would rebuild the
-- next access token from `users.org_id`/`users.role` (the caller's HOME org)
-- every time — silently reverting a switched session back to the wrong org
-- on its very next token refresh. NULL means "home org" (every refresh
-- token issued before this migration, and every ordinary login, behaves
-- exactly as before).
ALTER TABLE refresh_tokens ADD COLUMN org_id UUID REFERENCES organizations (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS org_id;
