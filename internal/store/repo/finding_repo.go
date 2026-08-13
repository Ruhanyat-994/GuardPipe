package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
)

// FindingRepo implements orchestrator.FindingRepository against the
// `findings` table (documentation/06-database-design.md §4.11) —
// read-only; every write goes through JobResultRepo instead (see its doc
// comment).
type FindingRepo struct {
	db Querier
}

func NewFindingRepo(db Querier) *FindingRepo {
	return &FindingRepo{db: db}
}

const findingSelectColumns = `
	SELECT id, scan_id, engine, rule_id, fingerprint, title, description, severity, confidence,
		cwe, cve, owasp, cvss_score, cvss_vector, location, remediation, status, metadata
	FROM findings`

func (r *FindingRepo) ListByScan(ctx context.Context, scanID uuid.UUID, page orchestrator.Page) ([]domain.Finding, int, error) {
	const countQ = `SELECT count(*) FROM findings WHERE scan_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, countQ, scanID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count findings: %w", err)
	}

	// Not a clamp-to-minimum (max() would wrongly force any smaller,
	// legitimate page_size like 10 up to 25) — this is "default when unset".
	pageSize := page.PageSize
	if pageSize < 1 {
		pageSize = 25
	}
	pageNum := page.Page
	if pageNum < 1 {
		pageNum = 1
	}

	q := findingSelectColumns + ` WHERE scan_id = $1 ORDER BY severity, created_at LIMIT $2 OFFSET $3`
	rows, err := r.db.Query(ctx, q, scanID, pageSize, (pageNum-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list findings: %w", err)
	}
	defer rows.Close()

	var out []domain.Finding
	for rows.Next() {
		f, err := findingRowScan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan finding row: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate findings: %w", err)
	}
	return out, total, nil
}

func (r *FindingRepo) CountByScanAndSeverity(ctx context.Context, scanID uuid.UUID) (map[domain.Severity]int, error) {
	const q = `SELECT severity, count(*) FROM findings WHERE scan_id = $1 GROUP BY severity`
	rows, err := r.db.Query(ctx, q, scanID)
	if err != nil {
		return nil, fmt.Errorf("repo: count findings by severity: %w", err)
	}
	defer rows.Close()

	counts := map[domain.Severity]int{}
	for rows.Next() {
		var severity string
		var n int
		if err := rows.Scan(&severity, &n); err != nil {
			return nil, fmt.Errorf("repo: scan severity count: %w", err)
		}
		counts[domain.Severity(severity)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate severity counts: %w", err)
	}
	return counts, nil
}

func (r *FindingRepo) CountByJob(ctx context.Context, jobID uuid.UUID) (int, error) {
	const q = `SELECT count(*) FROM findings WHERE job_id = $1`
	var n int
	if err := r.db.QueryRow(ctx, q, jobID).Scan(&n); err != nil {
		return 0, fmt.Errorf("repo: count findings by job: %w", err)
	}
	return n, nil
}

func findingRowScan(row pgx.Row) (domain.Finding, error) {
	var f domain.Finding
	var engine, severity, confidence, status string
	var locationJSON, metadataJSON []byte

	err := row.Scan(
		&f.ID, &f.ScanID, &engine, &f.RuleID, &f.Fingerprint, &f.Title, &f.Description,
		&severity, &confidence, &f.CWE, &f.CVE, &f.OWASP, &f.CVSSScore, &f.CVSSVector,
		&locationJSON, &f.Remediation, &status, &metadataJSON,
	)
	if err != nil {
		return domain.Finding{}, err
	}

	f.Engine = domain.EngineID(engine)
	f.Severity = domain.Severity(severity)
	f.Confidence = domain.Confidence(confidence)
	f.Status = domain.Status(status)

	if len(locationJSON) > 0 {
		if err := json.Unmarshal(locationJSON, &f.Location); err != nil {
			return domain.Finding{}, fmt.Errorf("decode location: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &f.Metadata); err != nil {
			return domain.Finding{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return f, nil
}
