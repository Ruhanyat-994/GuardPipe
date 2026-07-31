package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// CredentialRepo implements project.CredentialRepository against the
// `project_credentials` table (documentation/06-database-design.md §4.6).
// This is the **only** type in the codebase allowed to SELECT ciphertext —
// that rule is enforced by review, not the compiler, so keep every query
// touching that column inside this one file.
type CredentialRepo struct {
	db Querier
}

func NewCredentialRepo(db Querier) *CredentialRepo {
	return &CredentialRepo{db: db}
}

var _ project.CredentialRepository = (*CredentialRepo)(nil)

func (r *CredentialRepo) Upsert(ctx context.Context, row project.CredentialRow) (time.Time, error) {
	const q = `
		INSERT INTO project_credentials (id, project_id, kind, ciphertext, nonce, hint, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, kind) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			nonce = EXCLUDED.nonce,
			hint = EXCLUDED.hint,
			created_by = EXCLUDED.created_by,
			updated_at = now()
		RETURNING updated_at`
	var updatedAt time.Time
	err := r.db.QueryRow(ctx, q,
		id.New(), row.ProjectID, row.Kind, row.Ciphertext, row.Nonce, row.Hint, row.CreatedBy,
	).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("repo: upsert credential: %w", err)
	}
	return updatedAt, nil
}

func (r *CredentialRepo) GetInfo(ctx context.Context, projectID uuid.UUID, kind string) (*project.CredentialInfo, error) {
	const q = `SELECT hint, updated_at FROM project_credentials WHERE project_id = $1 AND kind = $2`
	var info project.CredentialInfo
	var updatedAt time.Time
	err := r.db.QueryRow(ctx, q, projectID, kind).Scan(&info.Hint, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("project.credential_not_found", "no credential stored for this project")
		}
		return nil, fmt.Errorf("repo: get credential info: %w", err)
	}
	info.HasCredential = true
	info.UpdatedAt = &updatedAt
	return &info, nil
}

// GetPlaintext decrypts the stored token. key is
// platform/config.Security.EncryptionKeyRaw, threaded through from the
// caller rather than held on this struct, so the key never sits in a field
// any other code could reach for.
func (r *CredentialRepo) GetPlaintext(ctx context.Context, projectID uuid.UUID, kind string, key []byte) (string, error) {
	const q = `SELECT ciphertext, nonce FROM project_credentials WHERE project_id = $1 AND kind = $2`
	var ciphertext, nonce []byte
	err := r.db.QueryRow(ctx, q, projectID, kind).Scan(&ciphertext, &nonce)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.NotFound("project.credential_not_found", "no credential stored for this project")
		}
		return "", fmt.Errorf("repo: get credential ciphertext: %w", err)
	}

	plaintext, err := crypto.Decrypt(key, ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("repo: decrypt credential: %w", err)
	}
	return string(plaintext), nil
}

func (r *CredentialRepo) Delete(ctx context.Context, projectID uuid.UUID, kind string) error {
	const q = `DELETE FROM project_credentials WHERE project_id = $1 AND kind = $2`
	tag, err := r.db.Exec(ctx, q, projectID, kind)
	if err != nil {
		return fmt.Errorf("repo: delete credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("project.credential_not_found", "no credential stored for this project")
	}
	return nil
}
