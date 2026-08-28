//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func TestFindingRepo_GetByID_ReturnsFindingWithEvidence(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	seedRule(t, pool, "codescan.injection.sql-string-concat")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, RuleID: "codescan.injection.sql-string-concat",
		Fingerprint: "fp-detail", Title: "sql injection", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "app.py", LineStart: 10},
		Remediation: "use parameterised queries", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: "query = f\"SELECT * FROM users WHERE id={id}\""}},
	}
	_, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	got, err := findings.GetByID(ctx, finding.ID)
	require.NoError(t, err)
	require.Equal(t, "sql injection", got.Title)
	require.Equal(t, domain.StatusOpen, got.Status)
	require.Nil(t, got.StatusChangedBy, "a freshly-inserted finding has never been triaged")
	require.Nil(t, got.StatusChangedAt)
	require.Len(t, got.Evidence, 1)
	require.Equal(t, "query = f\"SELECT * FROM users WHERE id={id}\"", got.Evidence[0].Value)
}

func TestFindingRepo_GetByID_UnknownID_ReturnsNotFound(t *testing.T) {
	pool := setupTestDB(t)
	findings := repo.NewFindingRepo(pool)

	_, err := findings.GetByID(context.Background(), id.New())
	require.Error(t, err)
}

func TestFindingStatusRepo_UpdateStatus_WritesStatusAndHistoryRow(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, actorID := seedOrgAndUser(t, pool)
	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectID, orgID, "Test Project")
	require.NoError(t, err)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	statusRepo := repo.NewFindingStatusRepo(pool)
	seedRule(t, pool, "depscan.secrets.hardcoded-key")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.secrets.hardcoded-key",
		Fingerprint: "fp-triage", Title: "t", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "config.py"},
		Remediation: "rotate the key", Status: domain.StatusOpen,
	}
	_, err = jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	reason := "this is a false positive from a test fixture value"
	err = statusRepo.UpdateStatus(ctx, finding.ID, domain.StatusOpen, domain.StatusFalsePositive, reason, actorID)
	require.NoError(t, err)

	findings := repo.NewFindingRepo(pool)
	got, err := findings.GetByID(ctx, finding.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusFalsePositive, got.Status)
	require.NotNil(t, got.StatusReason)
	require.Equal(t, reason, *got.StatusReason)
	require.NotNil(t, got.StatusChangedBy)
	require.Equal(t, actorID, *got.StatusChangedBy)
	require.NotNil(t, got.StatusChangedAt)

	history, err := statusRepo.ListHistory(ctx, finding.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, domain.StatusOpen, history[0].FromStatus)
	require.Equal(t, domain.StatusFalsePositive, history[0].ToStatus)
	require.Equal(t, reason, history[0].Reason)
	require.NotNil(t, history[0].ChangedBy)
	require.Equal(t, actorID, *history[0].ChangedBy)
}

// TestFindingStatusRepo_UpdateStatus_ConcurrentTransition_ReturnsConflict
// proves the optimistic-concurrency guard: a second UpdateStatus call that
// expects the finding to still be in its old `from` status, after another
// call already moved it, must fail loudly rather than silently overwrite.
func TestFindingStatusRepo_UpdateStatus_ConcurrentTransition_ReturnsConflict(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, actorID := seedOrgAndUser(t, pool)
	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectID, orgID, "Test Project")
	require.NoError(t, err)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	statusRepo := repo.NewFindingStatusRepo(pool)
	seedRule(t, pool, "codescan.injection.example")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineCodeScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	finding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, RuleID: "codescan.injection.example",
		Fingerprint: "fp-race", Title: "t", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "a.go"},
		Remediation: "r", Status: domain.StatusOpen,
	}
	_, err = jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)

	require.NoError(t, statusRepo.UpdateStatus(ctx, finding.ID, domain.StatusOpen, domain.StatusAcknowledged, "", actorID))

	// This call still thinks the finding is `open` — it isn't any more.
	err = statusRepo.UpdateStatus(ctx, finding.ID, domain.StatusOpen, domain.StatusFalsePositive, "", actorID)
	require.ErrorIs(t, err, reporting.ErrStatusConflict)
}

func TestFindingStatusRepo_ListHistory_UnknownFinding_ReturnsEmptyNotError(t *testing.T) {
	pool := setupTestDB(t)
	statusRepo := repo.NewFindingStatusRepo(pool)

	history, err := statusRepo.ListHistory(context.Background(), id.New())
	require.NoError(t, err)
	require.Empty(t, history)
}
