package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
)

// PlatformOperatorRepo implements admin.PlatformOperatorRepository against
// the `platform_operators` table
// (internal/store/migrations/00017_admin_platform_operators.sql). Grant/
// Revoke are deliberately not part of admin.PlatformOperatorRepository —
// see this type's own Grant/Revoke doc comments, and admin.Service's own
// doc comment, for why those two are CLI-only (cmd/guardpipe/admin.go),
// never reachable through the HTTP API.
type PlatformOperatorRepo struct {
	db Querier
}

func NewPlatformOperatorRepo(db Querier) *PlatformOperatorRepo {
	return &PlatformOperatorRepo{db: db}
}

var _ admin.PlatformOperatorRepository = (*PlatformOperatorRepo)(nil)

func (r *PlatformOperatorRepo) IsOperator(ctx context.Context, userID uuid.UUID) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM platform_operators WHERE user_id = $1)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("repo: check platform operator: %w", err)
	}
	return exists, nil
}

// Grant is called only from cmd/guardpipe/admin.go's `guardpipe admin
// grant-operator` subcommand — never exposed through admin.Service or any
// HTTP route (the anti-escalation control this package's own doc comment
// describes).
func (r *PlatformOperatorRepo) Grant(ctx context.Context, userID uuid.UUID, grantedBy *uuid.UUID, note string) (*admin.OperatorGrant, error) {
	const q = `
		INSERT INTO platform_operators (user_id, granted_by, note)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, granted_by, note, created_at`
	var g admin.OperatorGrant
	err := r.db.QueryRow(ctx, q, userID, grantedBy, note).
		Scan(&g.ID, &g.UserID, &g.GrantedBy, &g.Note, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("repo: grant platform operator: %w", err)
	}
	return &g, nil
}

// Revoke is called only from `guardpipe admin revoke-operator` — same
// reasoning as Grant.
func (r *PlatformOperatorRepo) Revoke(ctx context.Context, userID uuid.UUID) error {
	const q = `DELETE FROM platform_operators WHERE user_id = $1`
	tag, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("repo: revoke platform operator: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errNotAnOperator
	}
	return nil
}

var errNotAnOperator = errors.New("user is not a platform operator")

// ErrNotAnOperator is Revoke's not-found signal — cmd/guardpipe/admin.go
// checks it with errors.Is to print a clean CLI message instead of a raw
// "0 rows affected."
var ErrNotAnOperator = errNotAnOperator
