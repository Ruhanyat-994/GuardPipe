-- +goose Up
-- BUILD_GUIDE.md Phase 15. The invite token itself is never stored in
-- plaintext — only its SHA-256 hash — the same pattern `refresh_tokens`
-- already established for its own token hash (migration 00002).

CREATE TYPE invite_status AS ENUM ('pending', 'accepted', 'expired', 'revoked');

CREATE TABLE organization_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email       CITEXT NOT NULL,
    role        user_role NOT NULL DEFAULT 'member',
    invited_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    status      invite_status NOT NULL DEFAULT 'pending',
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_org_invites_org ON organization_invites (org_id, status);
CREATE INDEX idx_org_invites_email ON organization_invites (email, status);

-- +goose Down
DROP TABLE IF EXISTS organization_invites;
DROP TYPE IF EXISTS invite_status;
