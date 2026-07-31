package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ProjectRepo implements project.ProjectRepository against the `projects`
// table (documentation/06-database-design.md §4.4).
type ProjectRepo struct {
	db Querier
}

func NewProjectRepo(db Querier) *ProjectRepo {
	return &ProjectRepo{db: db}
}

var _ project.ProjectRepository = (*ProjectRepo)(nil)

func (r *ProjectRepo) Create(ctx context.Context, p *project.Project) error {
	const q = `
		INSERT INTO projects (id, org_id, name, description, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`
	err := r.db.QueryRow(ctx, q, p.ID, p.OrgID, p.Name, p.Description, string(p.Status), p.CreatedBy).
		Scan(&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert project: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetByID(ctx context.Context, id uuid.UUID) (*project.Project, error) {
	const q = `
		SELECT id, org_id, name, description, status, created_by, created_at, updated_at
		FROM projects WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *ProjectRepo) GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*project.Project, error) {
	const q = `
		SELECT id, org_id, name, description, status, created_by, created_at, updated_at
		FROM projects WHERE org_id = $1 AND name = $2`
	return r.scanOne(ctx, q, orgID, name)
}

func (r *ProjectRepo) scanOne(ctx context.Context, q string, args ...any) (*project.Project, error) {
	var p project.Project
	var status string
	err := r.db.QueryRow(ctx, q, args...).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &status, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.not_found", "project not found")
		}
		return nil, fmt.Errorf("repo: get project: %w", err)
	}
	p.Status = project.Status(status)
	return &p, nil
}

func (r *ProjectRepo) List(ctx context.Context, orgID uuid.UUID, page project.Page) ([]project.Project, int, error) {
	const countQ = `SELECT count(*) FROM projects WHERE org_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, countQ, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count projects: %w", err)
	}

	const listQ = `
		SELECT id, org_id, name, description, status, created_by, created_at, updated_at
		FROM projects WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, orgID, page.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list projects: %w", err)
	}
	defer rows.Close()

	var out []project.Project
	for rows.Next() {
		var p project.Project
		var status string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &status, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("repo: scan project: %w", err)
		}
		p.Status = project.Status(status)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate projects: %w", err)
	}
	return out, total, nil
}

func (r *ProjectRepo) Update(ctx context.Context, p *project.Project) error {
	const q = `
		UPDATE projects SET name = $2, description = $3, status = $4, updated_at = now()
		WHERE id = $1
		RETURNING updated_at`
	err := r.db.QueryRow(ctx, q, p.ID, p.Name, p.Description, string(p.Status)).Scan(&p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("project.not_found", "project not found")
		}
		return fmt.Errorf("repo: update project: %w", err)
	}
	return nil
}

func (r *ProjectRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM projects WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("repo: delete project: %w", err)
	}
	return nil
}
