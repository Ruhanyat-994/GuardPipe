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
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ScanJobRepo implements orchestrator.ScanJobRepository against the
// `scan_jobs` table (documentation/06-database-design.md §4.10). Like
// ScanRepo, there is no MarkSucceeded/MarkFailed/MarkSkipped here — those
// terminal writes go through JobResultRepo's one transaction instead.
type ScanJobRepo struct {
	db Querier
}

func NewScanJobRepo(db Querier) *ScanJobRepo {
	return &ScanJobRepo{db: db}
}

// CreateMany inserts one row per job with a plain loop rather than a
// pgx.Batch — a scan has at most seven jobs (one per engine), well below
// where batching would matter, and it keeps this repo on the same narrow
// Querier interface (Exec/Query/QueryRow) every other repo in this package
// uses.
func (r *ScanJobRepo) CreateMany(ctx context.Context, jobs []domain.ScanJob) error {
	const q = `INSERT INTO scan_jobs (id, scan_id, engine, status) VALUES ($1, $2, $3, $4)`
	for _, j := range jobs {
		if _, err := r.db.Exec(ctx, q, j.ID, j.ScanID, string(j.Engine), string(j.Status)); err != nil {
			return fmt.Errorf("repo: insert scan job: %w", err)
		}
	}
	return nil
}

func (r *ScanJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScanJob, error) {
	const q = jobSelectColumns + ` FROM scan_jobs WHERE id = $1`
	return jobRowScan(r.db.QueryRow(ctx, q, id))
}

func (r *ScanJobRepo) ListByScan(ctx context.Context, scanID uuid.UUID) ([]domain.ScanJob, error) {
	const q = jobSelectColumns + ` FROM scan_jobs WHERE scan_id = $1 ORDER BY engine`
	rows, err := r.db.Query(ctx, q, scanID)
	if err != nil {
		return nil, fmt.Errorf("repo: list scan jobs: %w", err)
	}
	defer rows.Close()

	var out []domain.ScanJob
	for rows.Next() {
		j, err := jobRowScan(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan job row: %w", err)
		}
		out = append(out, *j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate scan jobs: %w", err)
	}
	return out, nil
}

func (r *ScanJobRepo) MarkRunning(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE scan_jobs SET status = 'running', claimed_at = now(), started_at = now() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("repo: mark job running: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("job.not_found", "job not found")
	}
	return nil
}

var _ admin.EngineJobStatsReader = (*ScanJobRepo)(nil)

// EngineJobStatsSince satisfies admin.EngineJobStatsReader — real
// success/failure/skip counts per engine over the given window, for
// `GET /admin/system-health` (BUILD_GUIDE.md Phase 14). Always available:
// every engine writes to scan_jobs regardless of whether Docker/Redis/AI
// are reachable, unlike the other SystemHealth signals.
func (r *ScanJobRepo) EngineJobStatsSince(ctx context.Context, since time.Time) ([]admin.EngineJobStats, error) {
	const q = `
		SELECT engine,
			count(*) FILTER (WHERE status = 'succeeded') AS succeeded,
			count(*) FILTER (WHERE status = 'failed')    AS failed,
			count(*) FILTER (WHERE status = 'skipped')   AS skipped
		FROM scan_jobs
		WHERE claimed_at >= $1
		GROUP BY engine
		ORDER BY engine`
	rows, err := r.db.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("repo: engine job stats: %w", err)
	}
	defer rows.Close()

	var out []admin.EngineJobStats
	for rows.Next() {
		var s admin.EngineJobStats
		var engine string
		if err := rows.Scan(&engine, &s.Succeeded, &s.Failed, &s.Skipped); err != nil {
			return nil, fmt.Errorf("repo: scan engine job stats: %w", err)
		}
		s.Engine = domain.EngineID(engine)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate engine job stats: %w", err)
	}
	return out, nil
}

const jobSelectColumns = `
	SELECT id, scan_id, engine, status, attempt, claimed_at, started_at, finished_at,
		error_reason, skip_reason, stats`

func jobRowScan(row pgx.Row) (*domain.ScanJob, error) {
	var j domain.ScanJob
	var engine, status string
	var stats map[string]any

	err := row.Scan(
		&j.ID, &j.ScanID, &engine, &status, &j.Attempt, &j.ClaimedAt, &j.StartedAt, &j.FinishedAt,
		&j.ErrorReason, &j.SkipReason, &stats,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("job.not_found", "job not found")
		}
		return nil, err
	}
	j.Engine = domain.EngineID(engine)
	j.Status = domain.JobStatus(status)
	j.Stats = stats
	return &j, nil
}
