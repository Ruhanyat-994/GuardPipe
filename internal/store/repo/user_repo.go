package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// UserRepo implements identity.UserRepository against the `users` table
// (documentation/06-database-design.md §4.2). The interface lives with its
// consumer (internal/modules/identity); this is the implementation
// (documentation/04-backend-architecture.md §5.1).
type UserRepo struct {
	db Querier
}

func NewUserRepo(db Querier) *UserRepo {
	return &UserRepo{db: db}
}

var _ identity.UserRepository = (*UserRepo)(nil)

func (r *UserRepo) Create(ctx context.Context, u *identity.User) error {
	const q = `
		INSERT INTO users (id, org_id, email, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`
	err := r.db.QueryRow(ctx, q, u.ID, u.OrgID, u.Email, u.DisplayName, u.PasswordHash, string(u.Role)).
		Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert user: %w", err)
	}
	return nil
}

func (r *UserRepo) CountAll(ctx context.Context) (int, error) {
	const q = `SELECT count(*) FROM users`
	var n int
	if err := r.db.QueryRow(ctx, q).Scan(&n); err != nil {
		return 0, fmt.Errorf("repo: count users: %w", err)
	}
	return n, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*identity.User, error) {
	const q = `
		SELECT id, org_id, email, display_name, password_hash, role,
		       last_login_at, failed_login_count, locked_until, created_at, updated_at
		FROM users WHERE email = $1`
	return r.scanOne(ctx, q, email)
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*identity.User, error) {
	const q = `
		SELECT id, org_id, email, display_name, password_hash, role,
		       last_login_at, failed_login_count, locked_until, created_at, updated_at
		FROM users WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *UserRepo) scanOne(ctx context.Context, q string, arg any) (*identity.User, error) {
	var u identity.User
	var role string
	err := r.db.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.PasswordHash, &role,
		&u.LastLoginAt, &u.FailedLoginCount, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("identity.user_not_found", "no such user")
		}
		return nil, fmt.Errorf("repo: get user: %w", err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}

func (r *UserRepo) SetFailedLogin(ctx context.Context, id uuid.UUID, count int, lockedUntil *time.Time) error {
	const q = `UPDATE users SET failed_login_count = $2, locked_until = $3 WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, count, lockedUntil); err != nil {
		return fmt.Errorf("repo: set failed login: %w", err)
	}
	return nil
}

func (r *UserRepo) RecordSuccessfulLogin(ctx context.Context, id uuid.UUID, loginAt time.Time) error {
	const q = `UPDATE users SET failed_login_count = 0, locked_until = NULL, last_login_at = $2 WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, loginAt); err != nil {
		return fmt.Errorf("repo: record successful login: %w", err)
	}
	return nil
}
