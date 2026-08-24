package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"

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
	pentestConfigJSON, err := marshalPentestConfig(s.PentestConfig)
	if err != nil {
		return fmt.Errorf("repo: encode pentest config: %w", err)
	}
	requestedIP, err := parseOptionalIP(s.RequestedFromIP)
	if err != nil {
		return fmt.Errorf("repo: scan requested IP %q is not a valid address: %w", s.RequestedFromIP, err)
	}

	const q = `
		INSERT INTO scans (id, project_id, triggered_by, requested_ip, type, status, requested_engines, branch, finding_counts, pentest_config, queued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::engine_id[], $8, $9, $10, now())
		RETURNING queued_at`
	err = r.db.QueryRow(ctx, q,
		s.ID, s.ProjectID, s.TriggeredBy, requestedIP, string(s.Type), string(s.Status),
		engineIDsToStrings(s.RequestedEngines), s.Branch, countsJSON, pentestConfigJSON,
	).Scan(&s.QueuedAt)
	if err != nil {
		return fmt.Errorf("repo: insert scan: %w", err)
	}

	// A genuinely separate statement, not a RETURNING subquery on the INSERT
	// above: Postgres takes a command's snapshot before its own effects
	// apply, so a self-referential count(*) inside that INSERT's own
	// RETURNING silently undercounts by exactly the row just inserted
	// (verified against a real database — it returned 0 for a project's
	// first scan, not 1). This second query runs after the INSERT has
	// committed, so it sees the row correctly.
	const countQ = `SELECT count(*) FROM scans WHERE project_id = $1`
	if err := r.db.QueryRow(ctx, countQ, s.ProjectID).Scan(&s.ScanNumber); err != nil {
		return fmt.Errorf("repo: count scans for scan_number: %w", err)
	}
	return nil
}

func (r *ScanRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Scan, error) {
	const q = `
		SELECT id, project_id, triggered_by, requested_ip, type, status, requested_engines, commit_sha, branch,
			cancel_requested, error_reason, queued_at, started_at, finished_at, finding_counts, pentest_config,
			(SELECT count(*) FROM scans s2 WHERE s2.project_id = scans.project_id AND s2.created_at <= scans.created_at)
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

	// scan_number is a window function over every row this project's WHERE
	// clause matches — it's computed before the outer ORDER BY/LIMIT trims
	// down to one page, so pagination never skews the numbering.
	const listQ = `
		SELECT id, project_id, triggered_by, requested_ip, type, status, requested_engines, commit_sha, branch,
			cancel_requested, error_reason, queued_at, started_at, finished_at, finding_counts, pentest_config,
			ROW_NUMBER() OVER (ORDER BY created_at ASC)
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

	// scan_number is partitioned per project — the global list mixes scans
	// from every project, but "Scan #N" still needs to mean "the Nth scan
	// of that particular project," never a shared cross-project counter.
	const listQ = `
		SELECT s.id, s.project_id, s.triggered_by, s.type, s.status, s.requested_engines, s.commit_sha, s.branch,
			s.cancel_requested, s.error_reason, s.queued_at, s.started_at, s.finished_at, s.finding_counts, p.name,
			ROW_NUMBER() OVER (PARTITION BY s.project_id ORDER BY s.created_at ASC)
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
			&s.QueuedAt, &s.StartedAt, &s.FinishedAt, &findingCounts, &projectName, &s.ScanNumber,
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

// MarkStarted is a no-op past the first call for a given scan — the
// `WHERE status = 'queued'` guard is what makes calling this on every job
// claim (worker.go) safe rather than racing a later job's completion
// (JobResultRepo's own terminal UPDATE) into re-opening an already-finished
// scan. Zero RowsAffected is the expected, common case (every job after the
// first for the same scan), not an error.
func (r *ScanRepo) MarkStarted(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE scans SET status = 'running', started_at = now() WHERE id = $1 AND status = 'queued'`
	if _, err := r.db.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("repo: mark scan started: %w", err)
	}
	return nil
}

func scanRowScan(row pgx.Row) (*domain.Scan, error) {
	var s domain.Scan
	var scanType, status string
	var requestedEngines []string
	var findingCounts map[string]int
	var pentestConfigJSON []byte
	var requestedIP *netip.Addr

	err := row.Scan(
		&s.ID, &s.ProjectID, &s.TriggeredBy, &requestedIP, &scanType, &status, &requestedEngines,
		&s.CommitSHA, &s.Branch, &s.CancelRequested, &s.ErrorReason,
		&s.QueuedAt, &s.StartedAt, &s.FinishedAt, &findingCounts, &pentestConfigJSON, &s.ScanNumber,
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
	if requestedIP != nil {
		s.RequestedFromIP = requestedIP.String()
	}
	s.PentestConfig, err = unmarshalPentestConfig(pentestConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("repo: decode pentest config: %w", err)
	}
	return &s, nil
}

// parseOptionalIP parses ip into a *netip.Addr for the nullable `INET`
// requested_ip column — nil (NULL) when ip is empty, matching every other
// optional field this repo writes, rather than erroring on the common case
// of a scan created without a capturable client IP.
func parseOptionalIP(ip string) (*netip.Addr, error) {
	if ip == "" {
		return nil, nil
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil, err
	}
	return &addr, nil
}

// marshalPentestConfig/unmarshalPentestConfig round-trip domain.Scan.PentestConfig
// through the nullable `pentest_config JSONB` column (migration 00014) — nil
// in, nil out, same as every other optional JSONB field in this repo.
func marshalPentestConfig(cfg *domain.PentestScanConfig) ([]byte, error) {
	if cfg == nil {
		return nil, nil
	}
	return json.Marshal(cfg)
}

func unmarshalPentestConfig(raw []byte) (*domain.PentestScanConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cfg domain.PentestScanConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
