package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// OrganizationRepo implements identity.OrganizationRepository against the
// `organizations` table (documentation/06-database-design.md §4.1).
type OrganizationRepo struct {
	db Querier
}

func NewOrganizationRepo(db Querier) *OrganizationRepo {
	return &OrganizationRepo{db: db}
}

var (
	_ identity.OrganizationRepository = (*OrganizationRepo)(nil)
	_ admin.OrganizationRepository    = (*OrganizationRepo)(nil)
	_ organization.OrganizationReader = (*OrganizationRepo)(nil)
)

// GetName satisfies organization.OrganizationReader (BUILD_GUIDE.md
// Phase 15) — the org-switcher's display name lookup, on the same struct
// that already implements identity.OrganizationRepository and
// admin.OrganizationRepository against this table.
func (r *OrganizationRepo) GetName(ctx context.Context, orgID uuid.UUID) (string, error) {
	const q = `SELECT name FROM organizations WHERE id = $1`
	var name string
	if err := r.db.QueryRow(ctx, q, orgID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.NotFound("organization.not_found", "organization not found")
		}
		return "", fmt.Errorf("repo: get organization name: %w", err)
	}
	return name, nil
}

// Create makes a brand-new organisation and returns its id. Every
// registration calls this once (internal/modules/identity/service.go) so
// each account gets its own isolated organisation — see the multi-tenancy
// fix described in PROGRESS-LOG.md; the previous single-shared-organisation
// model (GetSole/EnsureDefault) is gone, not just unused.
func (r *OrganizationRepo) Create(ctx context.Context, name string) (uuid.UUID, error) {
	const insert = `INSERT INTO organizations (id, name) VALUES ($1, $2) RETURNING id`
	var orgID uuid.UUID
	if err := r.db.QueryRow(ctx, insert, id.New(), name).Scan(&orgID); err != nil {
		return uuid.Nil, fmt.Errorf("repo: create organization: %w", err)
	}
	return orgID, nil
}

// GetSuspensionState satisfies identity.OrganizationRepository — reads
// organizations.suspended_at/suspended_reason (migration 00017), written
// only through SetSuspended below.
func (r *OrganizationRepo) GetSuspensionState(ctx context.Context, orgID uuid.UUID) (*time.Time, string, error) {
	const q = `SELECT suspended_at, suspended_reason FROM organizations WHERE id = $1`
	var suspendedAt *time.Time
	var reason *string
	if err := r.db.QueryRow(ctx, q, orgID).Scan(&suspendedAt, &reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", apperrors.NotFound("identity.organization_not_found", "organization not found")
		}
		return nil, "", fmt.Errorf("repo: get organization suspension state: %w", err)
	}
	if reason == nil {
		return suspendedAt, "", nil
	}
	return suspendedAt, *reason, nil
}

// --- admin.OrganizationRepository (BUILD_GUIDE.md Phase 14) ---
//
// This type already implements identity.OrganizationRepository against the
// same `organizations` table — one repository struct satisfying two
// modules' interfaces, the same pattern UserRepo.GetDisplayName already
// establishes for `project`.

func (r *OrganizationRepo) ListAll(ctx context.Context, search string, page admin.Page) ([]admin.OrganizationSummary, int, error) {
	where := ""
	args := []any{}
	if search != "" {
		args = append(args, "%"+search+"%")
		where = " WHERE o.name ILIKE $1"
	}

	countQ := "SELECT count(*) FROM organizations o" + where
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count organizations: %w", err)
	}

	listQ := organizationSummaryColumns + where +
		fmt.Sprintf(" ORDER BY o.created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, append(args, page.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list organizations: %w", err)
	}
	defer rows.Close()

	var out []admin.OrganizationSummary
	for rows.Next() {
		o, err := scanOrganizationSummary(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan organization: %w", err)
		}
		out = append(out, *o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate organizations: %w", err)
	}
	return out, total, nil
}

func (r *OrganizationRepo) GetByID(ctx context.Context, orgID uuid.UUID) (*admin.OrganizationSummary, error) {
	const q = organizationSummaryColumns + " WHERE o.id = $1"
	o, err := scanOrganizationSummary(r.db.QueryRow(ctx, q, orgID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("admin.organization_not_found", "organization not found")
		}
		return nil, fmt.Errorf("repo: get organization: %w", err)
	}
	return o, nil
}

func (r *OrganizationRepo) SetSuspended(ctx context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error {
	const q = `UPDATE organizations SET suspended_at = $2, suspended_reason = $3 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, suspendedAt, reason)
	if err != nil {
		return fmt.Errorf("repo: set organization suspended: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("admin.organization_not_found", "organization not found")
	}
	return nil
}

// organizationSummaryColumns computes member/project/scan counts inline —
// the same "a repository is free to read across tables for a display-only
// aggregate" precedent GET /scans (org-wide) already uses for its
// project_name/finding_counts columns. scans has no org_id column of its
// own (denormalising it wasn't needed until this cross-org read existed),
// so its count joins through projects.
const organizationSummaryColumns = `
	SELECT o.id, o.name, o.suspended_at, o.suspended_reason, o.created_at,
		(SELECT count(*) FROM users u WHERE u.org_id = o.id) AS member_count,
		(SELECT count(*) FROM projects p WHERE p.org_id = o.id) AS project_count,
		(SELECT count(*) FROM scans s JOIN projects p ON p.id = s.project_id WHERE p.org_id = o.id) AS scan_count
	FROM organizations o`

func scanOrganizationSummary(row rowScanner) (*admin.OrganizationSummary, error) {
	var o admin.OrganizationSummary
	if err := row.Scan(
		&o.ID, &o.Name, &o.SuspendedAt, &o.SuspendedReason, &o.CreatedAt,
		&o.MemberCount, &o.ProjectCount, &o.ScanCount,
	); err != nil {
		return nil, err
	}
	return &o, nil
}
