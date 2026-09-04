package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// ProjectAssignmentRepo implements project.ProjectAssignmentRepository
// against `project_assignments` (migration 00021, BUILD_GUIDE.md Phase 15).
type ProjectAssignmentRepo struct {
	db Querier
}

func NewProjectAssignmentRepo(db Querier) *ProjectAssignmentRepo {
	return &ProjectAssignmentRepo{db: db}
}

var _ project.ProjectAssignmentRepository = (*ProjectAssignmentRepo)(nil)

func (r *ProjectAssignmentRepo) Create(ctx context.Context, a *project.ProjectAssignment) error {
	const q = `
		INSERT INTO project_assignments (id, project_id, user_id, assigned_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id, user_id) DO UPDATE SET assigned_by = EXCLUDED.assigned_by
		RETURNING assigned_at`
	aID := a.ID
	if aID == uuid.Nil {
		aID = id.New()
	}
	if err := r.db.QueryRow(ctx, q, aID, a.ProjectID, a.UserID, a.AssignedBy).Scan(&a.AssignedAt); err != nil {
		return fmt.Errorf("repo: insert project assignment: %w", err)
	}
	a.ID = aID
	return nil
}

func (r *ProjectAssignmentRepo) Delete(ctx context.Context, projectID, userID uuid.UUID) error {
	const q = `DELETE FROM project_assignments WHERE project_id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, q, projectID, userID)
	if err != nil {
		return fmt.Errorf("repo: delete project assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("project.assignment_not_found", "assignment not found")
	}
	return nil
}

const assignmentColumns = `SELECT id, project_id, user_id, assigned_by, assigned_at`

func (r *ProjectAssignmentRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]project.ProjectAssignment, error) {
	const q = assignmentColumns + ` FROM project_assignments WHERE project_id = $1 ORDER BY assigned_at`
	return r.list(ctx, q, projectID)
}

// ListByOrg backs the Team Dashboard — every assignment across every one of
// orgID's projects, joined through `projects` the same way
// organizationSummaryColumns already joins scans through projects for its
// own cross-table aggregate.
func (r *ProjectAssignmentRepo) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]project.ProjectAssignment, error) {
	const q = `
		SELECT pa.id, pa.project_id, pa.user_id, pa.assigned_by, pa.assigned_at
		FROM project_assignments pa
		JOIN projects p ON p.id = pa.project_id
		WHERE p.org_id = $1
		ORDER BY pa.assigned_at`
	return r.list(ctx, q, orgID)
}

func (r *ProjectAssignmentRepo) list(ctx context.Context, q string, arg any) ([]project.ProjectAssignment, error) {
	rows, err := r.db.Query(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("repo: list project assignments: %w", err)
	}
	defer rows.Close()

	var out []project.ProjectAssignment
	for rows.Next() {
		var a project.ProjectAssignment
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.UserID, &a.AssignedBy, &a.AssignedAt); err != nil {
			return nil, fmt.Errorf("repo: scan project assignment: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate project assignments: %w", err)
	}
	return out, nil
}
