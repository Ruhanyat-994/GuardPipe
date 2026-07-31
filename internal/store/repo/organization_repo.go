package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
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
