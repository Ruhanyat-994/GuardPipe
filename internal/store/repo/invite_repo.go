package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// InviteRepo implements organization.InviteRepository against
// `organization_invites` (migration 00019, BUILD_GUIDE.md Phase 15).
type InviteRepo struct {
	db Querier
}

func NewInviteRepo(db Querier) *InviteRepo {
	return &InviteRepo{db: db}
}

var _ organization.InviteRepository = (*InviteRepo)(nil)

func (r *InviteRepo) Create(ctx context.Context, in *organization.Invite) error {
	const q = `
		INSERT INTO organization_invites (id, org_id, email, role, invited_by, token_hash, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	invID := in.ID
	if invID == uuid.Nil {
		invID = id.New()
	}
	err := r.db.QueryRow(ctx, q, invID, in.OrgID, in.Email, string(in.Role), in.InvitedBy, in.TokenHash, string(in.Status), in.ExpiresAt).
		Scan(&in.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert organization invite: %w", err)
	}
	in.ID = invID
	return nil
}

const inviteColumns = `SELECT id, org_id, email, role, invited_by, token_hash, status, expires_at, created_at`

func (r *InviteRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*organization.Invite, error) {
	const q = inviteColumns + ` FROM organization_invites WHERE token_hash = $1`
	inv, err := scanInvite(r.db.QueryRow(ctx, q, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("organization.invite_not_found", "invite not found")
		}
		return nil, fmt.Errorf("repo: get organization invite by token: %w", err)
	}
	return inv, nil
}

func (r *InviteRepo) GetByID(ctx context.Context, id uuid.UUID) (*organization.Invite, error) {
	const q = inviteColumns + ` FROM organization_invites WHERE id = $1`
	inv, err := scanInvite(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("organization.invite_not_found", "invite not found")
		}
		return nil, fmt.Errorf("repo: get organization invite: %w", err)
	}
	return inv, nil
}

func (r *InviteRepo) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]organization.Invite, error) {
	const q = inviteColumns + ` FROM organization_invites WHERE org_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("repo: list organization invites: %w", err)
	}
	defer rows.Close()

	var out []organization.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan organization invite: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate organization invites: %w", err)
	}
	return out, nil
}

func (r *InviteRepo) ListByEmail(ctx context.Context, email string) ([]organization.Invite, error) {
	const q = inviteColumns + ` FROM organization_invites WHERE email = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q, email)
	if err != nil {
		return nil, fmt.Errorf("repo: list organization invites by email: %w", err)
	}
	defer rows.Close()

	var out []organization.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan organization invite: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate organization invites by email: %w", err)
	}
	return out, nil
}

func (r *InviteRepo) ExistsPending(ctx context.Context, orgID uuid.UUID, email string) (bool, error) {
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM organization_invites
			WHERE org_id = $1 AND email = $2 AND status = 'pending' AND expires_at > now()
		)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, orgID, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("repo: check pending organization invite: %w", err)
	}
	return exists, nil
}

func (r *InviteRepo) SetStatus(ctx context.Context, id uuid.UUID, status organization.InviteStatus) error {
	const q = `UPDATE organization_invites SET status = $2 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, string(status))
	if err != nil {
		return fmt.Errorf("repo: set organization invite status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("organization.invite_not_found", "invite not found")
	}
	return nil
}

func scanInvite(row rowScanner) (*organization.Invite, error) {
	var inv organization.Invite
	var role, status string
	if err := row.Scan(&inv.ID, &inv.OrgID, &inv.Email, &role, &inv.InvitedBy, &inv.TokenHash, &status, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
		return nil, err
	}
	inv.Role = domain.Role(role)
	inv.Status = organization.InviteStatus(status)
	return &inv, nil
}
