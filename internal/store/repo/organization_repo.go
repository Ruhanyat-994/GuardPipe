package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
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

var _ identity.OrganizationRepository = (*OrganizationRepo)(nil)

// GetSole returns the id of the single organisation for this release. It
// does not create one — see EnsureDefault, called once at startup.
func (r *OrganizationRepo) GetSole(ctx context.Context) (uuid.UUID, error) {
	const q = `SELECT id FROM organizations ORDER BY created_at LIMIT 1`
	var orgID uuid.UUID
	if err := r.db.QueryRow(ctx, q).Scan(&orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apperrors.NotFound("identity.organization_not_found", "no organisation exists yet")
		}
		return uuid.Nil, fmt.Errorf("repo: get sole organization: %w", err)
	}
	return orgID, nil
}

// EnsureDefault guarantees exactly one organisation exists, creating it with
// name if none does yet. Called once at startup (cmd/guardpipe/main.go) so
// registration never has to resolve a create-race between concurrent first
// requests.
func (r *OrganizationRepo) EnsureDefault(ctx context.Context, name string) (uuid.UUID, error) {
	if orgID, err := r.GetSole(ctx); err == nil {
		return orgID, nil
	} else if !isNotFoundErr(err) {
		return uuid.Nil, err
	}

	const insert = `INSERT INTO organizations (id, name) VALUES ($1, $2) RETURNING id`
	var orgID uuid.UUID
	if err := r.db.QueryRow(ctx, insert, id.New(), name).Scan(&orgID); err != nil {
		return uuid.Nil, fmt.Errorf("repo: create default organization: %w", err)
	}
	return orgID, nil
}

func isNotFoundErr(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
