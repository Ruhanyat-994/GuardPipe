package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/tx"
)

// JobResultRepo implements orchestrator.JobResultRepository — the one
// atomic write path for a completed job (documentation/04-backend-architecture.md
// §5.2: "one transaction per job completion"). Takes a *pgxpool.Pool
// directly (via tx.Beginner) rather than the narrower Querier every other
// repo in this package uses, since this is the one repository that starts
// its own transaction rather than composing inside a caller's.
type JobResultRepo struct {
	db tx.Beginner
}

func NewJobResultRepo(db *pgxpool.Pool) *JobResultRepo {
	return &JobResultRepo{db: db}
}

var _ orchestrator.JobResultRepository = (*JobResultRepo)(nil)

// terminalJobStatuses matches domain.JobStatus's terminal set — used to
// decide whether every job for a scan is now done.
var terminalJobStatuses = map[domain.JobStatus]bool{
	domain.JobStatusSucceeded: true, domain.JobStatusFailed: true,
	domain.JobStatusSkipped: true, domain.JobStatusCancelled: true,
}

func (r *JobResultRepo) PersistJobResult(ctx context.Context, result orchestrator.JobResult) error {
	return tx.WithTx(ctx, r.db, func(pgxTx pgx.Tx) error {
		if err := updateJobStatus(ctx, pgxTx, result); err != nil {
			return err
		}
		if err := insertFindings(ctx, pgxTx, result); err != nil {
			return err
		}
		return finalizeScanIfComplete(ctx, pgxTx, result.ScanID)
	})
}

func updateJobStatus(ctx context.Context, pgxTx pgx.Tx, result orchestrator.JobResult) error {
	statsJSON, err := json.Marshal(nonNilMap(result.Stats))
	if err != nil {
		return fmt.Errorf("repo: encode job stats: %w", err)
	}
	const q = `
		UPDATE scan_jobs SET status = $2, finished_at = now(), error_reason = $3, skip_reason = $4, stats = $5
		WHERE id = $1`
	_, err = pgxTx.Exec(ctx, q, result.JobID, string(result.Status), nullIfEmpty(result.ErrorReason), nullIfEmpty(result.SkipReason), statsJSON)
	if err != nil {
		return fmt.Errorf("repo: update job status: %w", err)
	}
	return nil
}

// insertFindings inserts each finding, idempotent on (scan_id,
// fingerprint) — a re-run of the same job never double-counts
// (NFR-REL-002). Evidence rows are only inserted for findings that
// actually landed (RowsAffected() == 1), since a conflicted finding's ID
// never entered the table and finding_evidence.finding_id is a foreign
// key — inserting evidence for a discarded finding would violate it.
func insertFindings(ctx context.Context, pgxTx pgx.Tx, result orchestrator.JobResult) error {
	if len(result.Findings) == 0 {
		return nil
	}

	const findingQ = `
		INSERT INTO findings (id, scan_id, job_id, project_id, engine, rule_id, source, fingerprint,
			title, description, severity, confidence, cwe, cve, owasp, cvss_score, cvss_vector,
			location, remediation, status, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (scan_id, fingerprint) DO NOTHING`

	const evidenceQ = `
		INSERT INTO finding_evidence (finding_id, kind, content, content_redacted, line_start, line_end, ordinal)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	for _, f := range result.Findings {
		locationJSON, err := json.Marshal(f.Location)
		if err != nil {
			return fmt.Errorf("repo: encode finding location: %w", err)
		}
		metadataJSON, err := json.Marshal(nonNilAnyMap(f.Metadata))
		if err != nil {
			return fmt.Errorf("repo: encode finding metadata: %w", err)
		}

		tag, err := pgxTx.Exec(ctx, findingQ,
			f.ID, result.ScanID, result.JobID, result.ProjectID, string(result.Engine), f.RuleID, string(f.Source.Effective()), f.Fingerprint,
			f.Title, f.Description, string(f.Severity), string(f.Confidence),
			nonNilStrings(f.CWE), nonNilStrings(f.CVE), nonNilStrings(f.OWASP),
			f.CVSSScore, f.CVSSVector, locationJSON, f.Remediation, string(domain.StatusOpen), metadataJSON,
		)
		if err != nil {
			return fmt.Errorf("repo: insert finding: %w", err)
		}
		if tag.RowsAffected() == 0 {
			continue // idempotent re-run: this finding already exists from a previous attempt
		}

		for i, ev := range f.Evidence {
			_, err := pgxTx.Exec(ctx, evidenceQ, f.ID, string(ev.Kind), ev.Value, ev.Redacted, nullIfZero(ev.LineStart), nullIfZero(ev.LineEnd), i)
			if err != nil {
				return fmt.Errorf("repo: insert finding evidence: %w", err)
			}
		}
	}
	return nil
}

// finalizeScanIfComplete checks whether every job for scanID is now
// terminal and, if so, computes finding_counts and writes the scan's own
// terminal status — inside the same transaction as the job/findings write
// above, matching documentation/04-backend-architecture.md §5.2 exactly.
func finalizeScanIfComplete(ctx context.Context, pgxTx pgx.Tx, scanID uuid.UUID) error {
	rows, err := pgxTx.Query(ctx, `SELECT status FROM scan_jobs WHERE scan_id = $1`, scanID)
	if err != nil {
		return fmt.Errorf("repo: list job statuses: %w", err)
	}
	var statuses []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return fmt.Errorf("repo: scan job status: %w", err)
		}
		statuses = append(statuses, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("repo: iterate job statuses: %w", err)
	}

	for _, s := range statuses {
		if !terminalJobStatuses[domain.JobStatus(s)] {
			return nil // at least one job still in flight
		}
	}

	var cancelRequested bool
	if err := pgxTx.QueryRow(ctx, `SELECT cancel_requested FROM scans WHERE id = $1`, scanID).Scan(&cancelRequested); err != nil {
		return fmt.Errorf("repo: get scan cancel_requested: %w", err)
	}

	countRows, err := pgxTx.Query(ctx, `SELECT severity, count(*) FROM findings WHERE scan_id = $1 GROUP BY severity`, scanID)
	if err != nil {
		return fmt.Errorf("repo: count findings by severity: %w", err)
	}
	counts := map[string]int{}
	for countRows.Next() {
		var severity string
		var n int
		if err := countRows.Scan(&severity, &n); err != nil {
			countRows.Close()
			return fmt.Errorf("repo: scan severity count: %w", err)
		}
		counts[severity] = n
	}
	countRows.Close()
	if err := countRows.Err(); err != nil {
		return fmt.Errorf("repo: iterate severity counts: %w", err)
	}

	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return fmt.Errorf("repo: encode finding counts: %w", err)
	}

	finalStatus := domain.ScanStatusCompleted
	if cancelRequested {
		finalStatus = domain.ScanStatusCancelled
	}
	_, err = pgxTx.Exec(ctx, `UPDATE scans SET status = $2, finished_at = now(), finding_counts = $3 WHERE id = $1`,
		scanID, string(finalStatus), countsJSON)
	if err != nil {
		return fmt.Errorf("repo: finalise scan: %w", err)
	}
	return nil
}

// nonNilStrings defaults a nil slice to empty — the TEXT[] columns this
// backs (cwe/cve/owasp) are NOT NULL DEFAULT '{}', but that default only
// applies when a column is omitted from the INSERT entirely; supplying an
// explicit nil parameter sends SQL NULL instead and violates the
// constraint.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonNilAnyMap(m map[string]any) map[string]any {
	return nonNilMap(m)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullIfZero(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
