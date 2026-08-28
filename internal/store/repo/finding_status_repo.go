package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/tx"
)

// FindingStatusRepo implements reporting.FindingStatusRepository — the
// triage write path, deliberately separate from FindingRepo (which stays
// read-only; see that type's own doc comment) since this is a genuinely
// different write path than the bulk finding-creation
// orchestrator.JobResultRepository owns. Takes a concrete *pgxpool.Pool
// (not the narrower Querier/Beginner interfaces every other repo in this
// package uses) because it needs both capabilities: UpdateStatus starts
// its own transaction (tx.Beginner), ListHistory is a plain read
// (Querier) — the same shape JobResultRepo's own doc comment explains.
type FindingStatusRepo struct {
	db *pgxpool.Pool
}

func NewFindingStatusRepo(db *pgxpool.Pool) *FindingStatusRepo {
	return &FindingStatusRepo{db: db}
}

var _ reporting.FindingStatusRepository = (*FindingStatusRepo)(nil)

// UpdateStatus updates findings.status/status_reason/status_changed_by/
// status_changed_at and appends one finding_status_history row, atomically.
// The `WHERE status = $from` guard is optimistic concurrency: if the row's
// status no longer matches what the caller last read (a concurrent
// transition already happened), RowsAffected is 0 and this returns
// reporting.ErrStatusConflict rather than silently overwriting a decision
// someone else just made.
func (r *FindingStatusRepo) UpdateStatus(ctx context.Context, findingID uuid.UUID, from, to domain.Status, reason string, changedBy uuid.UUID) error {
	return tx.WithTx(ctx, r.db, func(pgxTx pgx.Tx) error {
		reasonArg := nullIfEmpty(reason)

		tag, err := pgxTx.Exec(ctx, `
			UPDATE findings
			SET status = $2, status_reason = $3, status_changed_by = $4, status_changed_at = now(), updated_at = now()
			WHERE id = $1 AND status = $5`,
			findingID, string(to), reasonArg, changedBy, string(from))
		if err != nil {
			return fmt.Errorf("repo: update finding status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return reporting.ErrStatusConflict
		}

		_, err = pgxTx.Exec(ctx, `
			INSERT INTO finding_status_history (finding_id, from_status, to_status, reason, changed_by)
			VALUES ($1, $2, $3, $4, $5)`,
			findingID, string(from), string(to), reasonArg, changedBy)
		if err != nil {
			return fmt.Errorf("repo: insert finding status history: %w", err)
		}
		return nil
	})
}

func (r *FindingStatusRepo) ListHistory(ctx context.Context, findingID uuid.UUID) ([]reporting.StatusHistoryEntry, error) {
	const q = `
		SELECT from_status, to_status, reason, changed_by, changed_at
		FROM finding_status_history
		WHERE finding_id = $1
		ORDER BY changed_at DESC`
	rows, err := r.db.Query(ctx, q, findingID)
	if err != nil {
		return nil, fmt.Errorf("repo: list finding status history: %w", err)
	}
	defer rows.Close()

	var out []reporting.StatusHistoryEntry
	for rows.Next() {
		var e reporting.StatusHistoryEntry
		var from, to string
		var reason *string
		if err := rows.Scan(&from, &to, &reason, &e.ChangedBy, &e.ChangedAt); err != nil {
			return nil, fmt.Errorf("repo: scan finding status history row: %w", err)
		}
		e.FromStatus, e.ToStatus = domain.Status(from), domain.Status(to)
		if reason != nil {
			e.Reason = *reason
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate finding status history: %w", err)
	}
	return out, nil
}
