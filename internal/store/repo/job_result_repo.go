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

// PersistJobResult's bool return is true exactly when this call was the one
// that flipped the scan to a terminal status — i.e. every job for it is now
// done. A caller (Pool.persist) uses that signal to run scoring exactly
// once per scan, from the orchestrator layer rather than from inside this
// SQL-only transaction (CLAUDE.md: "Repository: SQL only... no business
// rules" — computing a risk score is business logic, unlike the
// finding_counts aggregate finalizeScanIfComplete already computes, which
// is a plain count, not a decision).
func (r *JobResultRepo) PersistJobResult(ctx context.Context, result orchestrator.JobResult) (bool, error) {
	var finalized bool
	err := tx.WithTx(ctx, r.db, func(pgxTx pgx.Tx) error {
		// Serialise every job result of one scan on the scan row. Without
		// this, jobs finishing at the same moment (a scan whose engines all
		// fail fast) each checked "are the others done?" before the others
		// had committed, each saw a sibling still running, and nobody
		// finalised the scan — it stayed queued/running forever with every
		// job terminal. With the lock, whichever commits last sees them all.
		if err := lockScan(ctx, pgxTx, result.ScanID); err != nil {
			return err
		}
		if err := updateJobStatus(ctx, pgxTx, result); err != nil {
			return err
		}
		if err := insertFindings(ctx, pgxTx, result); err != nil {
			return err
		}
		var err error
		finalized, err = finalizeScanIfComplete(ctx, pgxTx, result.ScanID)
		return err
	})
	return finalized, err
}

func lockScan(ctx context.Context, pgxTx pgx.Tx, scanID uuid.UUID) error {
	var id uuid.UUID
	if err := pgxTx.QueryRow(ctx, `SELECT id FROM scans WHERE id = $1 FOR UPDATE`, scanID).Scan(&id); err != nil {
		return fmt.Errorf("repo: lock scan: %w", err)
	}
	return nil
}

// FinalizeStuckScans repairs scans left non-terminal although every job is
// terminal (the pre-lock race above, or a crash between a job's write and
// its scan's). Returns the IDs it finalised, so the caller can score and
// notify exactly as if the last job had just finished.
func (r *JobResultRepo) FinalizeStuckScans(ctx context.Context) ([]uuid.UUID, error) {
	// The repo holds only a tx.Beginner, so even this read runs in a (short)
	// transaction.
	var candidates []uuid.UUID
	err := tx.WithTx(ctx, r.db, func(pgxTx pgx.Tx) error {
		rows, err := pgxTx.Query(ctx, `
			SELECT s.id FROM scans s
			WHERE s.status IN ('queued', 'running')
			  AND EXISTS (SELECT 1 FROM scan_jobs j WHERE j.scan_id = s.id)
			  AND NOT EXISTS (SELECT 1 FROM scan_jobs j WHERE j.scan_id = s.id AND j.status IN ('queued', 'running'))`)
		if err != nil {
			return fmt.Errorf("repo: list stuck scans: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("repo: scan stuck scan id: %w", err)
			}
			candidates = append(candidates, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	var finalized []uuid.UUID
	for _, scanID := range candidates {
		var done bool
		err := tx.WithTx(ctx, r.db, func(pgxTx pgx.Tx) error {
			if err := lockScan(ctx, pgxTx, scanID); err != nil {
				return err
			}
			var err error
			done, err = finalizeScanIfComplete(ctx, pgxTx, scanID)
			return err
		})
		if err != nil {
			return finalized, err
		}
		if done {
			finalized = append(finalized, scanID)
		}
	}
	return finalized, nil
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
// Returns whether it actually finalized the scan (false if a job is still
// in flight).
func finalizeScanIfComplete(ctx context.Context, pgxTx pgx.Tx, scanID uuid.UUID) (bool, error) {
	rows, err := pgxTx.Query(ctx, `SELECT status FROM scan_jobs WHERE scan_id = $1`, scanID)
	if err != nil {
		return false, fmt.Errorf("repo: list job statuses: %w", err)
	}
	var statuses []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return false, fmt.Errorf("repo: scan job status: %w", err)
		}
		statuses = append(statuses, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("repo: iterate job statuses: %w", err)
	}

	for _, s := range statuses {
		if !terminalJobStatuses[domain.JobStatus(s)] {
			return false, nil // at least one job still in flight
		}
	}

	var cancelRequested bool
	if err := pgxTx.QueryRow(ctx, `SELECT cancel_requested FROM scans WHERE id = $1`, scanID).Scan(&cancelRequested); err != nil {
		return false, fmt.Errorf("repo: get scan cancel_requested: %w", err)
	}

	countRows, err := pgxTx.Query(ctx, `SELECT severity, count(*) FROM findings WHERE scan_id = $1 GROUP BY severity`, scanID)
	if err != nil {
		return false, fmt.Errorf("repo: count findings by severity: %w", err)
	}
	counts := map[string]int{}
	for countRows.Next() {
		var severity string
		var n int
		if err := countRows.Scan(&severity, &n); err != nil {
			countRows.Close()
			return false, fmt.Errorf("repo: scan severity count: %w", err)
		}
		counts[severity] = n
	}
	countRows.Close()
	if err := countRows.Err(); err != nil {
		return false, fmt.Errorf("repo: iterate severity counts: %w", err)
	}

	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return false, fmt.Errorf("repo: encode finding counts: %w", err)
	}

	finalStatus := domain.ScanStatusCompleted
	if cancelRequested {
		finalStatus = domain.ScanStatusCancelled
	}
	// Only the first finaliser wins: a scan already terminal (finalised by an
	// earlier job, or by FinalizeStuckScans) is left alone and reports false,
	// so scoring and notifications run exactly once per scan.
	tag, err := pgxTx.Exec(ctx, `UPDATE scans SET status = $2, finished_at = now(), finding_counts = $3
		WHERE id = $1 AND status IN ('queued', 'running')`,
		scanID, string(finalStatus), countsJSON)
	if err != nil {
		return false, fmt.Errorf("repo: finalise scan: %w", err)
	}
	return tag.RowsAffected() == 1, nil
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
