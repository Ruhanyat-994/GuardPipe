-- +goose Up
-- BUILD_GUIDE.md Phase 15 follow-up ("project collaborators"). An org
-- invite (organization_invites/organization_memberships, migration
-- 00018/00019) grants org-wide access — every project, the Team Dashboard,
-- member management. project_assignments (migration 00021) is explicitly
-- *not* a permission grant, just a workload label for people who are
-- already org members. Neither covers "give one specific GuardPipe user
-- access to exactly one project, nothing else in this org" — that's what
-- project_invites/project_collaborators add. Deliberately its own pair of
-- tables rather than reusing project_assignments, so that table's existing
-- "not a permission grant" invariant stays true.
--
-- project_invites copies organization_invites' shape exactly (email,
-- token_hash, invite_status — the same enum, reused rather than
-- duplicated), scoped to a project instead of an org, plus the role being
-- granted (the inviter picks it, same as an org invite already lets them
-- pick a role).
CREATE TABLE project_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    email       CITEXT NOT NULL,
    role        user_role NOT NULL DEFAULT 'viewer',
    invited_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    status      invite_status NOT NULL DEFAULT 'pending',
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_project_invites_project ON project_invites (project_id, status);
CREATE INDEX idx_project_invites_email ON project_invites (email, status);

-- project_collaborators is the actual grant, created once an invite is
-- accepted — copies organization_memberships' shape, scoped to a project
-- instead of an org. A user_id row here never touches users.org_id or
-- organization_memberships; the grantee's own home org is completely
-- unaffected (CLAUDE.md's multi-tenancy rules).
CREATE TABLE project_collaborators (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role        user_role NOT NULL DEFAULT 'viewer',
    invited_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, user_id)
);

CREATE INDEX idx_project_collaborators_user ON project_collaborators (user_id);

-- refresh_tokens.project_id mirrors refresh_tokens.org_id (migration 00022)
-- exactly: nil for every ordinary session, set only when a caller switches
-- into a shared project (POST /auth/switch-project/{id}) — so Refresh can
-- rebuild the next access token against the same single-project scope
-- instead of silently widening back out to the whole org.
ALTER TABLE refresh_tokens ADD COLUMN project_id UUID REFERENCES projects (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS project_id;
DROP TABLE IF EXISTS project_collaborators;
DROP TABLE IF EXISTS project_invites;
