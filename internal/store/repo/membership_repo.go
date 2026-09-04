package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// MembershipRepo implements organization.MembershipRepository against
// `organization_memberships` (migration 00018, BUILD_GUIDE.md Phase 15) —
// and also identity.MembershipRoleReader against the same table, the same
// "one repo struct satisfies more than one module's interface" pattern
// UserRepo/OrganizationRepo already establish.
type MembershipRepo struct {
	db Querier
}

func NewMembershipRepo(db Querier) *MembershipRepo {
	return &MembershipRepo{db: db}
}

var (
	_ organization.MembershipRepository = (*MembershipRepo)(nil)
	_ identity.MembershipRoleReader     = (*MembershipRepo)(nil)
	_ project.MembershipChecker         = (*MembershipRepo)(nil)
	_ orchestrator.MembershipChecker    = (*MembershipRepo)(nil)
)

// IsOrgMember satisfies project.MembershipChecker (and, identically,
// orchestrator.MembershipChecker — BUILD_GUIDE.md Phase 15's project
// assignment / schedule assignee checks) — true when userID is either
// orgID's own home member (users.org_id) or holds an
// organization_memberships row for orgID. A free cross-table read, the same
// "a repository is free to read across tables for a display-only aggregate"
// precedent organizationSummaryColumns already establishes, just for a
// boolean instead of a count.
func (r *MembershipRepo) IsOrgMember(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM users WHERE id = $2 AND org_id = $1
			UNION
			SELECT 1 FROM organization_memberships WHERE org_id = $1 AND user_id = $2
		)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, orgID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("repo: check org membership: %w", err)
	}
	return exists, nil
}

func (r *MembershipRepo) Create(ctx context.Context, m *organization.Membership) error {
	const q = `
		INSERT INTO organization_memberships (id, org_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`
	mID := m.ID
	if mID == uuid.Nil {
		mID = id.New()
	}
	err := r.db.QueryRow(ctx, q, mID, m.OrgID, m.UserID, string(m.Role), m.InvitedBy).Scan(&m.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert organization membership: %w", err)
	}
	m.ID = mID
	return nil
}

const membershipColumns = `SELECT id, org_id, user_id, role, invited_by, created_at`

func (r *MembershipRepo) GetByOrgAndUser(ctx context.Context, orgID, userID uuid.UUID) (*organization.Membership, error) {
	const q = membershipColumns + ` FROM organization_memberships WHERE org_id = $1 AND user_id = $2`
	m, err := scanMembership(r.db.QueryRow(ctx, q, orgID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("organization.membership_not_found", "membership not found")
		}
		return nil, fmt.Errorf("repo: get organization membership: %w", err)
	}
	return m, nil
}

func (r *MembershipRepo) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]organization.Membership, error) {
	const q = membershipColumns + ` FROM organization_memberships WHERE org_id = $1 ORDER BY created_at`
	return r.list(ctx, q, orgID)
}

func (r *MembershipRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]organization.Membership, error) {
	const q = membershipColumns + ` FROM organization_memberships WHERE user_id = $1 ORDER BY created_at`
	return r.list(ctx, q, userID)
}

func (r *MembershipRepo) list(ctx context.Context, q string, arg any) ([]organization.Membership, error) {
	rows, err := r.db.Query(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("repo: list organization memberships: %w", err)
	}
	defer rows.Close()

	var out []organization.Membership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan organization membership: %w", err)
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate organization memberships: %w", err)
	}
	return out, nil
}

func (r *MembershipRepo) UpdateRole(ctx context.Context, orgID, userID uuid.UUID, role domain.Role) error {
	const q = `UPDATE organization_memberships SET role = $3 WHERE org_id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, q, orgID, userID, string(role))
	if err != nil {
		return fmt.Errorf("repo: update organization membership role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	return nil
}

func (r *MembershipRepo) Delete(ctx context.Context, orgID, userID uuid.UUID) error {
	const q = `DELETE FROM organization_memberships WHERE org_id = $1 AND user_id = $2`
	tag, err := r.db.Exec(ctx, q, orgID, userID)
	if err != nil {
		return fmt.Errorf("repo: delete organization membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	return nil
}

// GetRole satisfies identity.MembershipRoleReader — see that interface's
// own doc comment for why identity reads this table only through this one
// narrow method.
func (r *MembershipRepo) GetRole(ctx context.Context, orgID, userID uuid.UUID) (domain.Role, error) {
	const q = `SELECT role FROM organization_memberships WHERE org_id = $1 AND user_id = $2`
	var role string
	if err := r.db.QueryRow(ctx, q, orgID, userID).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.NotFound("organization.membership_not_found", "membership not found")
		}
		return "", fmt.Errorf("repo: get organization membership role: %w", err)
	}
	return domain.Role(role), nil
}

func scanMembership(row rowScanner) (*organization.Membership, error) {
	var m organization.Membership
	var role string
	if err := row.Scan(&m.ID, &m.OrgID, &m.UserID, &role, &m.InvitedBy, &m.CreatedAt); err != nil {
		return nil, err
	}
	m.Role = domain.Role(role)
	return &m, nil
}
