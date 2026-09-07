package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// ProjectCollaboratorRepo implements project.ProjectCollaboratorRepository
// against `project_collaborators` (migration 00024, project-collaborators
// follow-up) — and also identity.ProjectCollaboratorRoleReader against the
// same table, the same "one repo struct satisfies more than one module's
// interface" pattern MembershipRepo already establishes for org memberships.
type ProjectCollaboratorRepo struct {
	db Querier
}

func NewProjectCollaboratorRepo(db Querier) *ProjectCollaboratorRepo {
	return &ProjectCollaboratorRepo{db: db}
}

var (
	_ project.ProjectCollaboratorRepository = (*ProjectCollaboratorRepo)(nil)
	_ identity.ProjectCollaboratorRoleReader = (*ProjectCollaboratorRepo)(nil)
)

func (r *ProjectCollaboratorRepo) Create(ctx context.Context, c *project.ProjectCollaborator) error {
	const q = `
		INSERT INTO project_collaborators (id, project_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`
	cID := c.ID
	if cID == uuid.Nil {
		cID = id.New()
	}
	err := r.db.QueryRow(ctx, q, cID, c.ProjectID, c.UserID, string(c.Role), c.InvitedBy).Scan(&c.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert project collaborator: %w", err)
	}
	c.ID = cID
	return nil
}

const projectCollaboratorColumns = `SELECT id, project_id, user_id, role, invited_by, created_at`

func (r *ProjectCollaboratorRepo) GetByProjectAndUser(ctx context.Context, projectID, userID uuid.UUID) (*project.ProjectCollaborator, error) {
	const q = projectCollaboratorColumns + ` FROM project_collaborators WHERE project_id = $1 AND user_id = $2`
	c, err := scanProjectCollaborator(r.db.QueryRow(ctx, q, projectID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
		}
		return nil, fmt.Errorf("repo: get project collaborator: %w", err)
	}
	return c, nil
}

func (r *ProjectCollaboratorRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]project.ProjectCollaborator, error) {
	const q = projectCollaboratorColumns + ` FROM project_collaborators WHERE project_id = $1 ORDER BY created_at`
	return r.list(ctx, q, projectID)
}

func (r *ProjectCollaboratorRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]project.ProjectCollaborator, error) {
	const q = projectCollaboratorColumns + ` FROM project_collaborators WHERE user_id = $1 ORDER BY created_at`
	return r.list(ctx, q, userID)
}

func (r *ProjectCollaboratorRepo) list(ctx context.Context, q string, arg any) ([]project.ProjectCollaborator, error) {
	rows, err := r.db.Query(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("repo: list project collaborators: %w", err)
	}
	defer rows.Close()

	var out []project.ProjectCollaborator
	for rows.Next() {
		c, err := scanProjectCollaborator(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan project collaborator: %w", err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate project collaborators: %w", err)
	}
	return out, nil
}

func (r *ProjectCollaboratorRepo) Delete(ctx context.Context, projectID, userID uuid.UUID) error {
	const q = `DELETE FROM project_collaborators WHERE project_id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, q, projectID, userID)
	if err != nil {
		return fmt.Errorf("repo: delete project collaborator: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
	}
	return nil
}

// GetRoleAndOrg satisfies identity.ProjectCollaboratorRoleReader — see that
// interface's own doc comment for why identity reads this table only
// through this one narrow, joined method (it needs the project's owning org
// too, not just the role, to rebuild a Refresh'd access token).
func (r *ProjectCollaboratorRepo) GetRoleAndOrg(ctx context.Context, projectID, userID uuid.UUID) (domain.Role, uuid.UUID, error) {
	const q = `
		SELECT pc.role, p.org_id
		FROM project_collaborators pc
		JOIN projects p ON p.id = pc.project_id
		WHERE pc.project_id = $1 AND pc.user_id = $2`
	var role string
	var orgID uuid.UUID
	if err := r.db.QueryRow(ctx, q, projectID, userID).Scan(&role, &orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", uuid.Nil, apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
		}
		return "", uuid.Nil, fmt.Errorf("repo: get project collaborator role and org: %w", err)
	}
	return domain.Role(role), orgID, nil
}

func scanProjectCollaborator(row rowScanner) (*project.ProjectCollaborator, error) {
	var c project.ProjectCollaborator
	var role string
	if err := row.Scan(&c.ID, &c.ProjectID, &c.UserID, &role, &c.InvitedBy, &c.CreatedAt); err != nil {
		return nil, err
	}
	c.Role = domain.Role(role)
	return &c, nil
}
