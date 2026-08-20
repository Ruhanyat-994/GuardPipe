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

// DocumentRepo implements project.DocumentRepository against the
// `documents` table (migration 00013).
type DocumentRepo struct {
	db Querier
}

func NewDocumentRepo(db Querier) *DocumentRepo {
	return &DocumentRepo{db: db}
}

var _ project.DocumentRepository = (*DocumentRepo)(nil)

func (r *DocumentRepo) Create(ctx context.Context, d *project.Document) error {
	const q = `
		INSERT INTO documents (id, project_id, uploaded_by, filename, mime_type, size_bytes, content)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	err := r.db.QueryRow(ctx, q,
		d.ID, d.ProjectID, d.UploadedBy, d.Filename, d.MIMEType, d.SizeBytes, d.Content,
	).Scan(&d.ID, &d.CreatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert document: %w", err)
	}
	return nil
}

func (r *DocumentRepo) GetByID(ctx context.Context, id uuid.UUID) (*project.Document, error) {
	const q = `
		SELECT id, project_id, uploaded_by, filename, mime_type, size_bytes, content, created_at
		FROM documents WHERE id = $1`
	d, err := scanDocumentRow(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("document.not_found", "document not found")
		}
		return nil, fmt.Errorf("repo: get document: %w", err)
	}
	return d, nil
}

func (r *DocumentRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]project.Document, error) {
	const q = `
		SELECT id, project_id, uploaded_by, filename, mime_type, size_bytes, content, created_at
		FROM documents WHERE project_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q, projectID)
	if err != nil {
		return nil, fmt.Errorf("repo: list documents: %w", err)
	}
	defer rows.Close()

	var out []project.Document
	for rows.Next() {
		d, err := scanDocumentRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan document: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate documents: %w", err)
	}
	return out, nil
}

func (r *DocumentRepo) CountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	const q = `SELECT count(*) FROM documents WHERE project_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, q, projectID).Scan(&total); err != nil {
		return 0, fmt.Errorf("repo: count documents: %w", err)
	}
	return total, nil
}

func (r *DocumentRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM documents WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("repo: delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("document.not_found", "document not found")
	}
	return nil
}

func scanDocumentRow(row rowScanner) (*project.Document, error) {
	var d project.Document
	if err := row.Scan(&d.ID, &d.ProjectID, &d.UploadedBy, &d.Filename, &d.MIMEType, &d.SizeBytes, &d.Content, &d.CreatedAt); err != nil {
		return nil, err
	}
	return &d, nil
}
