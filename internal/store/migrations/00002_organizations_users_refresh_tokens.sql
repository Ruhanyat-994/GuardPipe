-- +goose Up
-- documentation/06-database-design.md §4.1-4.3.

CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email               CITEXT NOT NULL UNIQUE,
    display_name        TEXT NOT NULL,
    -- Argon2id encoded string — never a plaintext or reversible value.
    password_hash       TEXT NOT NULL,
    role                user_role NOT NULL DEFAULT 'member',
    last_login_at       TIMESTAMPTZ,
    failed_login_count  INT NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_org ON users (org_id);

CREATE TABLE refresh_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- SHA-256 of the token; the token itself is never stored.
    token_hash   TEXT NOT NULL UNIQUE,
    -- Rotation lineage — reuse detection invalidates the whole family.
    family_id    UUID NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    -- Non-null = already used; presenting it again is theft evidence.
    consumed_at  TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    user_agent   TEXT,
    ip           INET
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id);
CREATE INDEX idx_refresh_tokens_active ON refresh_tokens (user_id)
    WHERE revoked_at IS NULL AND consumed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
