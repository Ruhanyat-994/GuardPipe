package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ScanRepo implements orchestrator.ScanRepository against the `scans`
// table (documentation/06-database-design.md §4.9). Note there is no
// UpdateStatus method here — a scan's terminal status is only ever written
// as part of JobResultRepo's one atomic transaction (see job_result_repo.go).
type ScanRepo struct {
	db Querier
}

func NewScanRepo(db Querier) *ScanRepo {
	return &ScanRepo{db: db}
}

func (r *ScanRepo) Create(ctx context.Context, s *domain.Scan) error {
	countsJSON, err := json.Marshal(findingCountsToStringMap(s.FindingCounts))
	if err != nil {
		return fmt.Errorf("repo: encode finding counts: %w", err)
	}

	const q = `
		INSERT INTO scans (id, project_id, triggered_by, type, status, requested_engines, branch, finding_counts, queued_at)
		VALUES ($1, $2, $3, $4, $5, $6::engine_id[], $7, $8, now())
		RETURNING queued_at`
	err = r.db.QueryRow(ctx, q,
		s.ID, s.ProjectID, s.TriggeredBy, string(s.Type), string(s.Status),
		engineIDsToStrings(s.RequestedEngines), s.Branch, countsJSON,
	).Scan(&s.QueuedAt)
	if err != nil {
		return fmt.Errorf("repo: insert scan: %w", err)
	}
	return nil
}

func (r *ScanRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Scan, error) {
	const q = `
		SELECT id, project_id, triggered_by, type, status, requested_engines, commit_sha, branch,
			cancel_requested, error_reason, queued_at, started_at, finished_at, finding_counts
		FROM scans WHERE id = $1`
	return scanRowScan(r.db.QueryRow(ctx, q, id))
}

func (r *ScanRepo) SetCancelRequested(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE scans SET cancel_requested = true WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("repo: set scan cancel requested: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("scan.not_found", "scan not found")
	}
	return nil
}

func scanRowScan(row pgx.Row) (*domain.Scan, error) {
	var s domain.Scan
	var scanType, status string
	var requestedEngines []string
	var findingCounts map[string]int

	err := row.Scan(
		&s.ID, &s.ProjectID, &s.TriggeredBy, &scanType, &status, &requestedEngines,
		&s.CommitSHA, &s.Branch, &s.CancelRequested, &s.ErrorReason,
		&s.QueuedAt, &s.StartedAt, &s.FinishedAt, &findingCounts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("scan.not_found", "scan not found")
		}
		return nil, fmt.Errorf("repo: scan scan row: %w", err)
	}

	s.Type = domain.ScanType(scanType)
	s.Status = domain.ScanStatus(status)
	s.RequestedEngines = stringsToEngineIDs(requestedEngines)
	s.FindingCounts = stringMapToSeverityMap(findingCounts)
	return &s, nil
}

func engineIDsToStrings(ids []domain.EngineID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func stringsToEngineIDs(ss []string) []domain.EngineID {
	out := make([]domain.EngineID, len(ss))
	for i, s := range ss {
		out[i] = domain.EngineID(s)
	}
	return out
}

func findingCountsToStringMap(counts map[domain.Severity]int) map[string]int {
	out := make(map[string]int, len(counts))
	for k, v := range counts {
		out[string(k)] = v
	}
	return out
}

func stringMapToSeverityMap(m map[string]int) map[domain.Severity]int {
	out := make(map[domain.Severity]int, len(m))
	for k, v := range m {
		out[domain.Severity(k)] = v
	}
	return out
}
