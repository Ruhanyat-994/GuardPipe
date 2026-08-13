package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
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

// ListByProject powers the scan-history page — paginated, newest first,
// backed by idx_scans_project_created (documentation/06-database-design.md
// §4.9) rather than a full scan.
func (r *ScanRepo) ListByProject(ctx context.Context, projectID uuid.UUID, page orchestrator.Page) ([]domain.Scan, int, error) {
	const countQ = `SELECT count(*) FROM scans WHERE project_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, countQ, projectID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count scans: %w", err)
	}

	const listQ = `
		SELECT id, project_id, triggered_by, type, status, requested_engines, commit_sha, branch,
			cancel_requested, error_reason, queued_at, started_at, finished_at, finding_counts
		FROM scans WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, projectID, page.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list scans: %w", err)
	}
	defer rows.Close()

	var out []domain.Scan
	for rows.Next() {
		s, err := scanRowScan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan scan row: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate scans: %w", err)
	}
	return out, total, nil
}

// ListByOrg powers the global Scans page's cross-project history — every
// scan belonging to any of orgID's projects, newest first. Joins projects
// only to pull its name along; scoping itself is the WHERE clause, not a
// per-row ownership check (a scan whose project belongs to another org is
// never in the result set to begin with).
func (r *ScanRepo) ListByOrg(ctx context.Context, orgID uuid.UUID, page orchestrator.Page) ([]orchestrator.OrgScanSummary, int, error) {
	const countQ = `
		SELECT count(*) FROM scans s
		JOIN projects p ON p.id = s.project_id
		WHERE p.org_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, countQ, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count org scans: %w", err)
	}

	const listQ = `
		SELECT s.id, s.project_id, s.triggered_by, s.type, s.status, s.requested_engines, s.commit_sha, s.branch,
			s.cancel_requested, s.error_reason, s.queued_at, s.started_at, s.finished_at, s.finding_counts, p.name
		FROM scans s
		JOIN projects p ON p.id = s.project_id
		WHERE p.org_id = $1
		ORDER BY s.created_at DESC
		LIMIT $2 OFFSET $3`
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, orgID, page.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list org scans: %w", err)
	}
	defer rows.Close()

	var out []orchestrator.OrgScanSummary
	for rows.Next() {
		var s domain.Scan
		var scanType, status, projectName string
		var requestedEngines []string
		var findingCounts map[string]int

		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.TriggeredBy, &scanType, &status, &requestedEngines,
			&s.CommitSHA, &s.Branch, &s.CancelRequested, &s.ErrorReason,
			&s.QueuedAt, &s.StartedAt, &s.FinishedAt, &findingCounts, &projectName,
		); err != nil {
			return nil, 0, fmt.Errorf("repo: scan org scan row: %w", err)
		}
		s.Type = domain.ScanType(scanType)
		s.Status = domain.ScanStatus(status)
		s.RequestedEngines = stringsToEngineIDs(requestedEngines)
		s.FindingCounts = stringMapToSeverityMap(findingCounts)

		out = append(out, orchestrator.OrgScanSummary{Scan: s, ProjectName: projectName})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate org scans: %w", err)
	}
	return out, total, nil
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
