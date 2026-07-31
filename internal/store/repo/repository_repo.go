package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// RepositoryRepo implements project.RepositoryRepository against the
// `repositories` table (documentation/06-database-design.md §4.5).
type RepositoryRepo struct {
	db Querier
}

func NewRepositoryRepo(db Querier) *RepositoryRepo {
	return &RepositoryRepo{db: db}
}

var _ project.RepositoryRepository = (*RepositoryRepo)(nil)

func (r *RepositoryRepo) Upsert(ctx context.Context, repo *project.Repository) error {
	const q = `
		INSERT INTO repositories (id, project_id, provider, url, owner, name, default_branch, is_private, size_kb, last_validated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (project_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			url = EXCLUDED.url,
			owner = EXCLUDED.owner,
			name = EXCLUDED.name,
			default_branch = EXCLUDED.default_branch,
			is_private = EXCLUDED.is_private,
			size_kb = EXCLUDED.size_kb,
			last_validated_at = now()
		RETURNING id, last_validated_at`
	err := r.db.QueryRow(ctx, q,
		repo.ID, repo.ProjectID, repo.Provider, repo.URL, repo.Owner, repo.Name,
		repo.DefaultBranch, repo.IsPrivate, repo.SizeKB,
	).Scan(&repo.ID, &repo.LastValidatedAt)
	if err != nil {
		return fmt.Errorf("repo: upsert repository: %w", err)
	}
	return nil
}

func (r *RepositoryRepo) GetByProjectID(ctx context.Context, projectID uuid.UUID) (*project.Repository, error) {
	const q = `
		SELECT id, project_id, provider, url, owner, name, default_branch, is_private, size_kb, last_validated_at
		FROM repositories WHERE project_id = $1`
	var repository project.Repository
	err := r.db.QueryRow(ctx, q, projectID).Scan(
		&repository.ID, &repository.ProjectID, &repository.Provider, &repository.URL, &repository.Owner,
		&repository.Name, &repository.DefaultBranch, &repository.IsPrivate, &repository.SizeKB, &repository.LastValidatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.repository_not_found", "no repository attached to this project")
		}
		return nil, fmt.Errorf("repo: get repository: %w", err)
	}
	return &repository, nil
}
