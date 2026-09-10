package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// ProjectInviteRepo implements project.ProjectInviteRepository against
// `project_invites` (migration 00024, project-collaborators follow-up) —
// mirrors InviteRepo (organization_invites) exactly, one project instead of
// one org.
type ProjectInviteRepo struct {
	db Querier
}

func NewProjectInviteRepo(db Querier) *ProjectInviteRepo {
	return &ProjectInviteRepo{db: db}
}

var _ project.ProjectInviteRepository = (*ProjectInviteRepo)(nil)

func (r *ProjectInviteRepo) Create(ctx context.Context, in *project.ProjectInvite) error {
	const q = `
		INSERT INTO project_invites (id, project_id, email, role, invited_by, token_hash, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	invID := in.ID
	if invID == uuid.Nil {
		invID = id.New()
	}
	err := r.db.QueryRow(ctx, q, invID, in.ProjectID, in.Email, string(in.Role), in.InvitedBy, in.TokenHash, string(in.Status), in.ExpiresAt).
		Scan(&in.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert project invite: %w", err)
	}
	in.ID = invID
	return nil
}

const projectInviteColumns = `SELECT id, project_id, email, role, invited_by, token_hash, status, expires_at, created_at`

func (r *ProjectInviteRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*project.ProjectInvite, error) {
	const q = projectInviteColumns + ` FROM project_invites WHERE token_hash = $1`
	inv, err := scanProjectInvite(r.db.QueryRow(ctx, q, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.invite_not_found", "invite not found")
		}
		return nil, fmt.Errorf("repo: get project invite by token: %w", err)
	}
	return inv, nil
}

func (r *ProjectInviteRepo) GetByID(ctx context.Context, id uuid.UUID) (*project.ProjectInvite, error) {
	const q = projectInviteColumns + ` FROM project_invites WHERE id = $1`
	inv, err := scanProjectInvite(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.invite_not_found", "invite not found")
		}
		return nil, fmt.Errorf("repo: get project invite: %w", err)
	}
	return inv, nil
}

func (r *ProjectInviteRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]project.ProjectInvite, error) {
	const q = projectInviteColumns + ` FROM project_invites WHERE project_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("repo: list project invites: %w", err)
	}
	defer rows.Close()

	var out []project.ProjectInvite
	for rows.Next() {
		inv, err := scanProjectInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan project invite: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate project invites: %w", err)
	}
	return out, nil
}

func (r *ProjectInviteRepo) ListByEmail(ctx context.Context, email string) ([]project.ProjectInvite, error) {
	const q = projectInviteColumns + ` FROM project_invites WHERE email = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q, email)
	if err != nil {
		return nil, fmt.Errorf("repo: list project invites by email: %w", err)
	}
	defer rows.Close()

	var out []project.ProjectInvite
	for rows.Next() {
		inv, err := scanProjectInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan project invite: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate project invites by email: %w", err)
	}
	return out, nil
}

func (r *ProjectInviteRepo) ExistsPending(ctx context.Context, projectID uuid.UUID, email string) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM project_invites
			WHERE project_id = $1 AND email = $2 AND status = 'pending' AND expires_at > now()
		)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, projectID, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("repo: check pending project invite: %w", err)
	}
	return exists, nil
}

func (r *ProjectInviteRepo) SetStatus(ctx context.Context, id uuid.UUID, status project.InviteStatus) error {
	const q = `UPDATE project_invites SET status = $2 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, string(status))
	if err != nil {
		return fmt.Errorf("repo: set project invite status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("project.invite_not_found", "invite not found")
	}
	return nil
}

func scanProjectInvite(row rowScanner) (*project.ProjectInvite, error) {
	var inv project.ProjectInvite
	var role, status string
	if err := row.Scan(&inv.ID, &inv.ProjectID, &inv.Email, &role, &inv.InvitedBy, &inv.TokenHash, &status, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
		return nil, err
	}
	inv.Role = domain.Role(role)
	inv.Status = status
	return &inv, nil
}
