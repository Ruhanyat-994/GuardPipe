//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// seedProject creates the minimum row chain a scan needs: an org, a user,
// and a project (no repository attached — these tests exercise
// persistence, not cloning).
func seedProject(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, pool)

	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		projectID, orgID, "Test Project")
	require.NoError(t, err)
	return projectID
}

func TestScanRepo_CreateAndGet_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	branch := "main"
	scan := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain,
		Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan},
		Branch: &branch, FindingCounts: map[domain.Severity]int{},
	}
	require.NoError(t, scans.Create(ctx, scan))
	require.False(t, scan.QueuedAt.IsZero(), "Create() must populate QueuedAt from the DB default")

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, got.Status)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, got.RequestedEngines)
	require.Equal(t, "main", *got.Branch)
	require.False(t, got.CancelRequested)
}

func TestScanRepo_SetCancelRequested(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	require.NoError(t, scans.SetCancelRequested(ctx, scan.ID))

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.True(t, got.CancelRequested)
}

func TestScanJobRepo_CreateListAndMarkRunning(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))

	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	list, err := jobs.ListByScan(ctx, scan.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, domain.JobStatusQueued, list[0].Status)

	require.NoError(t, jobs.MarkRunning(ctx, job.ID))
	got, err := jobs.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusRunning, got.Status)
	require.NotNil(t, got.StartedAt)
	require.NotNil(t, got.ClaimedAt)
}

// TestJobResultRepo_PersistJobResult_OneJobScan_FindingsPersistAndScanFinalizes
// is the core correctness proof for documentation/04-backend-architecture.md
// §5.2's "one transaction per job completion": findings land, the job's
// status updates, and — since this is the scan's only job — the scan
// finalises to completed with the right finding_counts, all from one call.
func TestJobResultRepo_PersistJobResult_OneJobScan_FindingsPersistAndScanFinalizes(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)

	seedRule(t, pool, "depscan.hygiene.no-lockfile")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))
	require.NoError(t, jobs.MarkRunning(ctx, job.ID))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.no-lockfile",
		Fingerprint: "fp-1", Title: "no lockfile", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "package.json"},
		Remediation: "commit a lockfile", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindManifestExcerpt, Value: "no lockfile present"}},
	}

	err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Stats: map[string]any{"files_scanned": 1},
		Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	gotJob, err := jobs.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, gotJob.Status)
	require.NotNil(t, gotJob.FinishedAt)

	list, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, "depscan.hygiene.no-lockfile", list[0].RuleID)
	require.Equal(t, domain.LocationTypeFile, list[0].Location.Type)
	require.Equal(t, "package.json", list[0].Location.Path)

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, gotScan.Status, "the scan finalises once its only job is terminal")
	require.NotNil(t, gotScan.FinishedAt)
	require.Equal(t, 1, gotScan.FindingCounts[domain.SeverityMedium])
}

func TestJobResultRepo_PersistJobResult_IdempotentOnRerun(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	seedRule(t, pool, "depscan.hygiene.wildcard-version")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.wildcard-version",
		Fingerprint: "same-fingerprint", Title: "t", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "left-pad"},
		Remediation: "pin a version", Status: domain.StatusOpen,
	}
	result := orchestrator.JobResult{JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding}}

	require.NoError(t, jobResults.PersistJobResult(ctx, result))
	// Re-run with a fresh finding ID but the same fingerprint — simulating
	// the same job re-executing (NFR-REL-002: re-runs are idempotent).
	finding.ID = id.New()
	result.Findings = []domain.Finding{finding}
	require.NoError(t, jobResults.PersistJobResult(ctx, result))

	_, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total, "a re-run with the same fingerprint must not double-insert")
}

func TestJobResultRepo_PersistJobResult_MultiJobScan_DoesNotFinalizeUntilLastJob(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan, domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	jobA := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	jobB := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{jobA, jobB}))

	require.NoError(t, jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobA.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded}))

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, gotScan.Status, "must not finalise while jobB is still queued")

	require.NoError(t, jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobB.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan, Status: domain.JobStatusSkipped, SkipReason: "no manifest"}))

	gotScan, err = scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, gotScan.Status, "finalises once the last job (skipped counts as terminal) is done")
}

// seedRule inserts the minimum rules row a finding's rule_id foreign key
// needs — findings.rule_id references rules(id) ON DELETE RESTRICT.
func seedRule(t *testing.T, pool *pgxpool.Pool, ruleID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO rules (id, engine, category, title, description, remediation, default_severity, tier)
		VALUES ($1, 'depscan', 'test', 'test rule', 'test', 'test', 'medium', 'core')
		ON CONFLICT (id) DO NOTHING`, ruleID)
	require.NoError(t, err)
}
