package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ProjectWebhookRepo implements livescan.Repository against
// `project_webhooks` (migration 00027, BUILD_GUIDE.md Phase 17 Part B).
type ProjectWebhookRepo struct {
	db Querier
}

func NewProjectWebhookRepo(db Querier) *ProjectWebhookRepo {
	return &ProjectWebhookRepo{db: db}
}

var _ livescan.Repository = (*ProjectWebhookRepo)(nil)

func (r *ProjectWebhookRepo) Create(ctx context.Context, w *livescan.Webhook) error {
	const q = `
		INSERT INTO project_webhooks (id, project_id, github_hook_id, secret_ciphertext, secret_nonce, engines,
			watched_branches, enabled_by, attested_at, live_scan_min_balance_percent)
		VALUES ($1, $2, $3, $4, $5, $6::engine_id[], $7, $8, $9, $10)
		RETURNING created_at, updated_at`
	err := r.db.QueryRow(ctx, q, w.ID, w.ProjectID, w.GitHubHookID, w.SecretCiphertext, w.SecretNonce,
		engineIDsToStrings(w.Engines), w.WatchedBranches, w.EnabledBy, w.AttestedAt, w.MinBalancePercent,
	).Scan(&w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert project webhook: %w", err)
	}
	return nil
}

const projectWebhookColumns = `
	SELECT id, project_id, github_hook_id, secret_ciphertext, secret_nonce, engines, watched_branches,
	       enabled_by, attested_at, last_delivery_at, last_delivery_status, paused_reason, paused_at,
	       created_at, updated_at, live_scan_min_balance_percent
	FROM project_webhooks`

func (r *ProjectWebhookRepo) GetByID(ctx context.Context, id uuid.UUID) (*livescan.Webhook, error) {
	return r.get(ctx, projectWebhookColumns+` WHERE id = $1`, id)
}

func (r *ProjectWebhookRepo) GetByProjectID(ctx context.Context, projectID uuid.UUID) (*livescan.Webhook, error) {
	return r.get(ctx, projectWebhookColumns+` WHERE project_id = $1`, projectID)
}

func (r *ProjectWebhookRepo) get(ctx context.Context, q string, arg any) (*livescan.Webhook, error) {
	var w livescan.Webhook
	var engines []string
	var status *string
	err := r.db.QueryRow(ctx, q, arg).Scan(
		&w.ID, &w.ProjectID, &w.GitHubHookID, &w.SecretCiphertext, &w.SecretNonce, &engines, &w.WatchedBranches,
		&w.EnabledBy, &w.AttestedAt, &w.LastDeliveryAt, &status, &w.PausedReason, &w.PausedAt,
		&w.CreatedAt, &w.UpdatedAt, &w.MinBalancePercent,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("webhook.not_found", "webhook not found")
		}
		return nil, fmt.Errorf("repo: get project webhook: %w", err)
	}
	w.Engines = stringsToEngineIDs(engines)
	if status != nil {
		w.LastDeliveryStatus = *status
	}
	return &w, nil
}

func (r *ProjectWebhookRepo) UpdateConfig(ctx context.Context, w *livescan.Webhook) error {
	const q = `
		UPDATE project_webhooks
		SET engines = $2::engine_id[], watched_branches = $3, enabled_by = $4, attested_at = $5,
		    live_scan_min_balance_percent = $6, paused_reason = NULL, paused_at = NULL, updated_at = now()
		WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, w.ID, engineIDsToStrings(w.Engines), w.WatchedBranches, w.EnabledBy, w.AttestedAt, w.MinBalancePercent)
	if err != nil {
		return fmt.Errorf("repo: update project webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("webhook.not_found", "webhook not found")
	}
	return nil
}

func (r *ProjectWebhookRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM project_webhooks WHERE id = $1`, id); err != nil {
		return fmt.Errorf("repo: delete project webhook: %w", err)
	}
	return nil
}

func (r *ProjectWebhookRepo) RecordDelivery(ctx context.Context, id uuid.UUID, at time.Time, status string) error {
	const q = `UPDATE project_webhooks SET last_delivery_at = $2, last_delivery_status = $3, updated_at = now() WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, at, status); err != nil {
		return fmt.Errorf("repo: record project webhook delivery: %w", err)
	}
	return nil
}

func (r *ProjectWebhookRepo) Pause(ctx context.Context, id uuid.UUID, reason string, at time.Time) error {
	const q = `UPDATE project_webhooks SET paused_reason = $2, paused_at = $3, updated_at = now() WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, reason, at); err != nil {
		return fmt.Errorf("repo: pause project webhook: %w", err)
	}
	return nil
}
