package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
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

var (
	_ identity.UserRepository       = (*UserRepo)(nil)
	_ admin.UserRepository          = (*UserRepo)(nil)
	_ organization.UserReader       = (*UserRepo)(nil)
	_ project.UserDisplayNameLookup = (*UserRepo)(nil)
)

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
	const q = userSelectColumns + ` FROM users WHERE email = $1`
	return r.scanOne(ctx, q, email)
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*identity.User, error) {
	const q = userSelectColumns + ` FROM users WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

// userSelectColumns is shared by GetByEmail/GetByID — suspended_at/
// suspended_reason (migration 00017) are read here so identity.Service can
// reject a suspended account at Login/CheckSuspension without a second
// query; only modules/admin ever writes them (SetSuspended below).
const userSelectColumns = `
	SELECT id, org_id, email, display_name, password_hash, role,
	       last_login_at, failed_login_count, locked_until,
	       suspended_at, suspended_reason, created_at, updated_at`

// GetDisplayName satisfies project.UserDisplayNameLookup — `project` needs
// only this one field from `identity`'s tables, for a target attestation's
// "attested_by" (documentation/07-api-specification.md §4).
func (r *UserRepo) GetDisplayName(ctx context.Context, id uuid.UUID) (string, error) {
	const q = `SELECT display_name FROM users WHERE id = $1`
	var name string
	if err := r.db.QueryRow(ctx, q, id).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.NotFound("identity.user_not_found", "no such user")
		}
		return "", fmt.Errorf("repo: get display name: %w", err)
	}
	return name, nil
}

// GetEmail satisfies project.UserDisplayNameLookup's other method
// (project-collaborators follow-up) — the caller's own email, matched
// against a ProjectInvite's Email on accept/decline.
func (r *UserRepo) GetEmail(ctx context.Context, id uuid.UUID) (string, error) {
	const q = `SELECT email FROM users WHERE id = $1`
	var email string
	if err := r.db.QueryRow(ctx, q, id).Scan(&email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.NotFound("identity.user_not_found", "no such user")
		}
		return "", fmt.Errorf("repo: get email: %w", err)
	}
	return email, nil
}

func (r *UserRepo) scanOne(ctx context.Context, q string, arg any) (*identity.User, error) {
	var u identity.User
	var role string
	err := r.db.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.PasswordHash, &role,
		&u.LastLoginAt, &u.FailedLoginCount, &u.LockedUntil,
		&u.SuspendedAt, &u.SuspendedReason, &u.CreatedAt, &u.UpdatedAt,
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

// --- admin.UserRepository (BUILD_GUIDE.md Phase 14) ---
//
// This type already implements identity.UserRepository against the same
// `users` table — one repository struct satisfying two modules'
// interfaces, the same pattern GetDisplayName already establishes for
// `project`.

func (r *UserRepo) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]admin.UserSummary, error) {
	const q = userSummaryColumns + ` FROM users WHERE org_id = $1 ORDER BY created_at`
	rows, err := r.db.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("repo: list users by org: %w", err)
	}
	defer rows.Close()

	var out []admin.UserSummary
	for rows.Next() {
		u, err := scanUserSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan user summary: %w", err)
		}
		out = append(out, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate users by org: %w", err)
	}
	return out, nil
}

func (r *UserRepo) GetSummaryByID(ctx context.Context, id uuid.UUID) (*admin.UserSummary, error) {
	const q = userSummaryColumns + ` FROM users WHERE id = $1`
	u, err := scanUserSummary(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("admin.user_not_found", "user not found")
		}
		return nil, fmt.Errorf("repo: get user summary: %w", err)
	}
	return u, nil
}

func (r *UserRepo) SetSuspended(ctx context.Context, id uuid.UUID, suspendedAt *time.Time, reason *string) error {
	const q = `UPDATE users SET suspended_at = $2, suspended_reason = $3 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, suspendedAt, reason)
	if err != nil {
		return fmt.Errorf("repo: set user suspended: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("admin.user_not_found", "user not found")
	}
	return nil
}

const userSummaryColumns = `
	SELECT id, org_id, email, display_name, role, suspended_at, suspended_reason, last_login_at, created_at`

func scanUserSummary(row rowScanner) (*admin.UserSummary, error) {
	var u admin.UserSummary
	var role string
	if err := row.Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &role,
		&u.SuspendedAt, &u.SuspendedReason, &u.LastLoginAt, &u.CreatedAt,
	); err != nil {
		return nil, err
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

// --- organization.UserReader (BUILD_GUIDE.md Phase 15) ---
//
// This type already implements identity.UserRepository and
// admin.UserRepository against the same `users` table — one repository
// struct satisfying a third module's interface.

// GetInfoByID is named distinctly from identity.UserRepository's own
// GetByID (which returns a full identity.User) and admin.UserRepository's
// GetSummaryByID (a different DTO) — all three are implemented by this same
// struct, and Go doesn't allow two methods of the same name with different
// signatures on one type.
func (r *UserRepo) GetInfoByID(ctx context.Context, id uuid.UUID) (*organization.UserInfo, error) {
	const q = `SELECT id, org_id, email, display_name, role FROM users WHERE id = $1`
	u, err := scanUserInfo(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("organization.user_not_found", "user not found")
		}
		return nil, fmt.Errorf("repo: get user info: %w", err)
	}
	return u, nil
}

// ListHomeMembers returns every user whose own home org is orgID — see
// organization.UserReader's own doc comment on why this is read
// generically rather than assumed to be exactly one row.
func (r *UserRepo) ListHomeMembers(ctx context.Context, orgID uuid.UUID) ([]organization.UserInfo, error) {
	const q = `SELECT id, org_id, email, display_name, role FROM users WHERE org_id = $1 ORDER BY created_at`
	rows, err := r.db.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("repo: list home members: %w", err)
	}
	defer rows.Close()

	var out []organization.UserInfo
	for rows.Next() {
		u, err := scanUserInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan home member: %w", err)
		}
		out = append(out, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate home members: %w", err)
	}
	return out, nil
}

// SetHomeRole is the one write modules/organization makes against this
// package's own table — see organization.UserReader's own doc comment for
// why this narrow method is sanctioned.
func (r *UserRepo) SetHomeRole(ctx context.Context, id uuid.UUID, role domain.Role) error {
	const q = `UPDATE users SET role = $2 WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, string(role))
	if err != nil {
		return fmt.Errorf("repo: set home role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("organization.user_not_found", "user not found")
	}
	return nil
}

func scanUserInfo(row rowScanner) (*organization.UserInfo, error) {
	var u organization.UserInfo
	var role string
	if err := row.Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &role); err != nil {
		return nil, err
	}
	u.Role = domain.Role(role)
	return &u, nil
}
