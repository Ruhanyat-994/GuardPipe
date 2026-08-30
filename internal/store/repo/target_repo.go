package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// TargetRepo implements project.TargetRepository against the
// `pentest_targets` table (documentation/06-database-design.md §4.7).
type TargetRepo struct {
	db Querier
}

func NewTargetRepo(db Querier) *TargetRepo {
	return &TargetRepo{db: db}
}

var (
	_ project.TargetRepository = (*TargetRepo)(nil)
	_ admin.TargetReader       = (*TargetRepo)(nil)
)

func (r *TargetRepo) Create(ctx context.Context, t *project.Target) error {
	const q = `
		INSERT INTO pentest_targets (id, project_id, target, normalized_host, pinned_ips, status, last_resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`
	err := r.db.QueryRow(ctx, q,
		t.ID, t.ProjectID, t.TargetInput, t.NormalizedHost, t.PinnedIPs, string(t.Status), t.LastResolvedAt,
	).Scan(&t.ID)
	if err != nil {
		return fmt.Errorf("repo: insert pentest target: %w", err)
	}
	return nil
}

func (r *TargetRepo) GetByID(ctx context.Context, id uuid.UUID) (*project.Target, error) {
	const q = `
		SELECT id, project_id, target, normalized_host, pinned_ips, status, last_resolved_at
		FROM pentest_targets WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *TargetRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]project.Target, error) {
	const q = `
		SELECT id, project_id, target, normalized_host, pinned_ips, status, last_resolved_at
		FROM pentest_targets WHERE project_id = $1 ORDER BY last_resolved_at DESC`
	rows, err := r.db.Query(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("repo: list pentest targets: %w", err)
	}
	defer rows.Close()

	var out []project.Target
	for rows.Next() {
		t, err := scanTargetRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan pentest target: %w", err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate pentest targets: %w", err)
	}
	return out, nil
}

func (r *TargetRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status project.TargetStatus) error {
	const q = `UPDATE pentest_targets SET status = $2 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, string(status))
	if err != nil {
		return fmt.Errorf("repo: update pentest target status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("target.not_found", "pentest target not found")
	}
	return nil
}

// GetTargetInfo satisfies admin.TargetReader — a read-only join across
// project's `pentest_targets` and `projects` tables (a display read, not a
// business operation; see admin.TargetInfo's own doc comment for why this
// is fine here rather than routed through project.Service).
func (r *TargetRepo) GetTargetInfo(ctx context.Context, targetID uuid.UUID) (*admin.TargetInfo, error) {
	const q = `
		SELECT t.id, t.normalized_host, p.id, p.name, p.org_id, o.name
		FROM pentest_targets t
		JOIN projects p ON p.id = t.project_id
		JOIN organizations o ON o.id = p.org_id
		WHERE t.id = $1`
	var info admin.TargetInfo
	err := r.db.QueryRow(ctx, q, targetID).Scan(
		&info.TargetID, &info.Host, &info.ProjectID, &info.ProjectName, &info.OrgID, &info.OrgName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("target.not_found", "pentest target not found")
		}
		return nil, fmt.Errorf("repo: get target info: %w", err)
	}
	return &info, nil
}

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// so scanTargetRow works for both GetByID and ListByProject.
type rowScanner interface {
	Scan(dest ...any) error
}

func (r *TargetRepo) scanOne(ctx context.Context, q string, args ...any) (*project.Target, error) {
	t, err := scanTargetRow(r.db.QueryRow(ctx, q, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("target.not_found", "pentest target not found")
		}
		return nil, fmt.Errorf("repo: get pentest target: %w", err)
	}
	return t, nil
}

func scanTargetRow(row rowScanner) (*project.Target, error) {
	var t project.Target
	var status string
	if err := row.Scan(&t.ID, &t.ProjectID, &t.TargetInput, &t.NormalizedHost, &t.PinnedIPs, &status, &t.LastResolvedAt); err != nil {
		return nil, err
	}
	t.Status = project.TargetStatus(status)
	return &t, nil
}
