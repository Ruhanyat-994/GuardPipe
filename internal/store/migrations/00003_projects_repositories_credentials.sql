-- +goose Up
-- documentation/06-database-design.md §4.4-4.6.

CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    status      project_status NOT NULL DEFAULT 'active',
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE INDEX idx_projects_org_status ON projects (org_id, status, created_at DESC);

CREATE TABLE repositories (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         UUID NOT NULL UNIQUE REFERENCES projects (id) ON DELETE CASCADE,
    provider           TEXT NOT NULL CHECK (provider IN ('github')),
    -- Normalised HTTPS clone URL.
    url                TEXT NOT NULL,
    owner              TEXT NOT NULL,
    name               TEXT NOT NULL,
    default_branch     TEXT NOT NULL DEFAULT 'main',
    is_private         BOOLEAN NOT NULL DEFAULT false,
    -- From the GitHub API; used for the pre-clone size guard.
    size_kb            BIGINT,
    last_validated_at  TIMESTAMPTZ
);

-- Separate from repositories so credential rows can carry stricter access
-- control and a distinct audit trail.
CREATE TABLE project_credentials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('github_pat')),
    -- AES-256-GCM. No query outside this table's repository may select
    -- ciphertext (documentation/06-database-design.md §4.6).
    ciphertext  BYTEA NOT NULL,
    nonce       BYTEA NOT NULL,
    -- Masked display value, e.g. "ghp_••••3f9a" — the real token is never
    -- returned by any API response.
    hint        TEXT NOT NULL,
    created_by  UUID REFERENCES users (id),
    -- Not in the original documentation/06-database-design.md §4.6 table —
    -- added to match that document's own universal "every table gets
    -- created_at/updated_at" convention and the credential response shape
    -- documented in 07-api-specification.md (PUT .../credential returns
    -- "updated_at", which nothing else on this row could supply since
    -- "rotate" is a full row overwrite, not a new row). §4.6 has been
    -- updated to match in the same change.
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind)
);

-- +goose Down
DROP TABLE IF EXISTS project_credentials;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS projects;
