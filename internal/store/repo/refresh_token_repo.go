package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// RefreshTokenRepo implements identity.RefreshTokenRepository against the
// `refresh_tokens` table (documentation/06-database-design.md §4.3).
type RefreshTokenRepo struct {
	db Querier
}

func NewRefreshTokenRepo(db Querier) *RefreshTokenRepo {
	return &RefreshTokenRepo{db: db}
}

var _ identity.RefreshTokenRepository = (*RefreshTokenRepo)(nil)

func (r *RefreshTokenRepo) Create(ctx context.Context, rt *identity.RefreshToken) error {
	const q = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.Exec(ctx, q, rt.ID, rt.UserID, rt.TokenHash, rt.FamilyID, rt.ExpiresAt, rt.UserAgent, rt.IP)
	if err != nil {
		return fmt.Errorf("repo: insert refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*identity.RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, family_id, expires_at, consumed_at, revoked_at, user_agent, ip
		FROM refresh_tokens WHERE token_hash = $1`
	var rt identity.RefreshToken
	err := r.db.QueryRow(ctx, q, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.FamilyID, &rt.ExpiresAt,
		&rt.ConsumedAt, &rt.RevokedAt, &rt.UserAgent, &rt.IP,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("identity.token_not_found", "no such refresh token")
		}
		return nil, fmt.Errorf("repo: get refresh token: %w", err)
	}
	return &rt, nil
}

func (r *RefreshTokenRepo) MarkConsumed(ctx context.Context, id uuid.UUID, consumedAt time.Time) error {
	const q = `UPDATE refresh_tokens SET consumed_at = $2 WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, consumedAt); err != nil {
		return fmt.Errorf("repo: mark refresh token consumed: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepo) RevokeFamily(ctx context.Context, familyID uuid.UUID, revokedAt time.Time) error {
	const q = `UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1 AND revoked_at IS NULL`
	if _, err := r.db.Exec(ctx, q, familyID, revokedAt); err != nil {
		return fmt.Errorf("repo: revoke refresh token family: %w", err)
	}
	return nil
}
