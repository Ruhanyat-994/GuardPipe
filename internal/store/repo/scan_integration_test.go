//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
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
		Branch: &branch, FindingCounts: map[domain.Severity]int{}, RequestedFromIP: "203.0.113.10",
	}
	require.NoError(t, scans.Create(ctx, scan))
	require.False(t, scan.QueuedAt.IsZero(), "Create() must populate QueuedAt from the DB default")
	require.Equal(t, 1, scan.ScanNumber, "the first scan for a project must be numbered 1")

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, got.Status)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, got.RequestedEngines)
	require.Equal(t, "main", *got.Branch)
	require.False(t, got.CancelRequested)
	require.Equal(t, 1, got.ScanNumber, "GetByID must return the same scan_number Create did")
	require.Equal(t, "203.0.113.10", got.RequestedFromIP, "the scan's requesting IP must round-trip through the nullable INET column")
}

// TestScanRepo_CreateAndGet_NoRequestedIP is the near-miss half of the IP
// round-trip above: an empty RequestedFromIP must persist as SQL NULL and
// come back as an empty string, not error or a bogus "0.0.0.0"-style value.
func TestScanRepo_CreateAndGet_NoRequestedIP(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	scan := &domain.Scan{
		ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain,
		Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan},
		FindingCounts: map[domain.Severity]int{},
	}
	require.NoError(t, scans.Create(ctx, scan))

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, "", got.RequestedFromIP, "no IP was provided, so none should come back")
}

// TestScanRepo_ListByProject_NewestFirstAndScopedToProject exercises the
// scan-history query: newest-first ordering (idx_scans_project_created)
// and that a second project's scans never leak into the first's list.
// Both projects share one seeded org/user — seedOrgAndUser hardcodes a
// single email, so seeding it twice in one test would violate the users
// table's unique-email constraint.
func TestScanRepo_ListByProject_NewestFirstAndScopedToProject(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, _ := seedOrgAndUser(t, pool)

	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		projectID, orgID, "Test Project")
	require.NoError(t, err)

	otherProjectID := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`,
		otherProjectID, orgID, "Other Project")
	require.NoError(t, err)

	scans := repo.NewScanRepo(pool)

	first := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, first))
	second := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, second))
	other := &domain.Scan{ID: id.New(), ProjectID: otherProjectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, other))

	list, total, err := scans.ListByProject(ctx, projectID, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID, "newest scan first")
	require.Equal(t, first.ID, list[1].ID)

	otherList, otherTotal, err := scans.ListByProject(ctx, otherProjectID, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, otherTotal)
	require.Len(t, otherList, 1)
	require.Equal(t, other.ID, otherList[0].ID)
}

// TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst is the real-Postgres
// proof for the global Scans page's cross-project history: scans from two
// different projects in the SAME org both show up (newest first, with
// their project's name attached), and a second org's scan never leaks in —
// the org-scoping is the WHERE clause itself, not a filter applied after
// the fact.
func TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	scans := repo.NewScanRepo(pool)

	orgA, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org A")
	require.NoError(t, err)
	orgB, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org B")
	require.NoError(t, err)

	projectA1 := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectA1, orgA, "Org A Project 1")
	require.NoError(t, err)
	projectA2 := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectA2, orgA, "Org A Project 2")
	require.NoError(t, err)
	projectB := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectB, orgB, "Org B Project")
	require.NoError(t, err)

	first := &domain.Scan{ID: id.New(), ProjectID: projectA1, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, first))
	second := &domain.Scan{ID: id.New(), ProjectID: projectA2, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, second))
	otherOrgScan := &domain.Scan{ID: id.New(), ProjectID: projectB, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, otherOrgScan))

	list, total, err := scans.ListByOrg(ctx, orgA, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total, "both of org A's projects' scans count, org B's does not")
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID, "newest scan first, across projects")
	require.Equal(t, "Org A Project 2", list[0].ProjectName)
	require.Equal(t, first.ID, list[1].ID)
	require.Equal(t, "Org A Project 1", list[1].ProjectName)

	otherList, otherTotal, err := scans.ListByOrg(ctx, orgB, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, otherTotal)
	require.Len(t, otherList, 1)
	require.Equal(t, otherOrgScan.ID, otherList[0].ID)
	require.Equal(t, "Org B Project", otherList[0].ProjectName)
}

// TestScanRepo_ListActiveByOrg_OnlyQueuedAndRunningInOwnOrg: finished
// scans and another org's in-flight scan must not show up in the running-
// scans indicator, and "Scan #N" must still count the finished scans.
func TestScanRepo_ListActiveByOrg_OnlyQueuedAndRunningInOwnOrg(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	scans := repo.NewScanRepo(pool)

	orgA, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org A")
	require.NoError(t, err)
	orgB, err := repo.NewOrganizationRepo(pool).Create(ctx, "Org B")
	require.NoError(t, err)
	projectA := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectA, orgA, "Org A Project")
	require.NoError(t, err)
	projectB := id.New()
	_, err = pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, $3, 'active')`, projectB, orgB, "Org B Project")
	require.NoError(t, err)

	newScan := func(projectID uuid.UUID) *domain.Scan {
		s := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
		require.NoError(t, scans.Create(ctx, s))
		return s
	}
	done := newScan(projectA)
	_, err = pool.Exec(ctx, `UPDATE scans SET status = 'completed' WHERE id = $1`, done.ID)
	require.NoError(t, err)
	running := newScan(projectA)
	require.NoError(t, scans.MarkStarted(ctx, running.ID))
	queued := newScan(projectA)
	newScan(projectB)

	list, err := scans.ListActiveByOrg(ctx, orgA, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, queued.ID, list[0].ID, "newest first")
	require.Equal(t, 3, list[0].ScanNumber, "numbering counts the completed scan too")
	require.Equal(t, running.ID, list[1].ID)
	require.Equal(t, domain.ScanStatusRunning, list[1].Status)
	require.Equal(t, "Org A Project", list[1].ProjectName)

	limited, err := scans.ListActiveByOrg(ctx, orgA, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
}

func TestScanJobRepo_ListRunningStartedBefore(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, repo.NewScanRepo(pool).Create(ctx, scan))
	jobs := repo.NewScanJobRepo(pool)
	old, fresh, queued := id.New(), id.New(), id.New()
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{
		{ID: old, ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued},
		{ID: fresh, ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued},
		{ID: queued, ScanID: scan.ID, Engine: domain.EngineK8sScan, Status: domain.JobStatusQueued},
	}))
	require.NoError(t, jobs.MarkRunning(ctx, old))
	require.NoError(t, jobs.MarkRunning(ctx, fresh))
	_, err := pool.Exec(ctx, `UPDATE scan_jobs SET started_at = now() - interval '2 hours' WHERE id = $1`, old)
	require.NoError(t, err)

	stale, err := jobs.ListRunningStartedBefore(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, stale, 1)
	require.Equal(t, old, stale[0].ID)
}

// TestJobResultRepo_ConcurrentResults_FinalizeExactlyOnce reproduces the
// stuck-scan bug: seven engines failing at the same instant each committed
// "not all done yet" and the scan stayed queued forever. With the scan-row
// lock, exactly one of them finalises it.
func TestJobResultRepo_ConcurrentResults_FinalizeExactlyOnce(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, repo.NewScanRepo(pool).Create(ctx, scan))
	engines := []domain.EngineID{domain.EngineDocReview, domain.EngineCodeScan, domain.EngineDepScan, domain.EngineContainerScan, domain.EngineK8sScan, domain.EngineCICDScan, domain.EnginePentest}
	var jobs []domain.ScanJob
	for _, e := range engines {
		jobs = append(jobs, domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: e, Status: domain.JobStatusRunning})
	}
	require.NoError(t, repo.NewScanJobRepo(pool).CreateMany(ctx, jobs))

	results := repo.NewJobResultRepo(pool)
	var wg sync.WaitGroup
	var finalizedCount atomic.Int32
	start := make(chan struct{})
	for _, j := range jobs {
		wg.Add(1)
		go func(j domain.ScanJob) {
			defer wg.Done()
			<-start
			done, err := results.PersistJobResult(ctx, orchestrator.JobResult{JobID: j.ID, ScanID: scan.ID, ProjectID: projectID, Engine: j.Engine, Status: domain.JobStatusFailed, ErrorReason: "workspace_unavailable"})
			assert.NoError(t, err)
			if done {
				finalizedCount.Add(1)
			}
		}(j)
	}
	close(start)
	wg.Wait()

	require.Equal(t, int32(1), finalizedCount.Load(), "exactly one job result finalises the scan")
	got, err := repo.NewScanRepo(pool).GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, got.Status)

	// Repair path: a scan stuck by the old bug is found and closed once.
	stuck := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypePartial, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, repo.NewScanRepo(pool).Create(ctx, stuck))
	require.NoError(t, repo.NewScanJobRepo(pool).CreateMany(ctx, []domain.ScanJob{{ID: id.New(), ScanID: stuck.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusFailed}}))
	ids, err := results.FinalizeStuckScans(ctx)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{stuck.ID}, ids, "the already-completed scan is not touched again")
	ids, err = results.FinalizeStuckScans(ctx)
	require.NoError(t, err)
	require.Empty(t, ids)
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

func TestScanRepo_MarkStarted_SetsStatusAndTimestampOnce(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))

	require.NoError(t, scans.MarkStarted(ctx, scan.ID))

	got, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusRunning, got.Status)
	require.NotNil(t, got.StartedAt)
	firstStartedAt := *got.StartedAt

	// Near-miss: a second call (every job after the first one claimed for
	// this scan, worker.go) must be a no-op — it must not overwrite
	// StartedAt with a later time or revert a status a concurrent job
	// completion has since moved to something terminal.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, scans.MarkStarted(ctx, scan.ID))

	got2, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusRunning, got2.Status)
	require.WithinDuration(t, firstStartedAt, *got2.StartedAt, 0, "MarkStarted must not update StartedAt once already set")
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

	finalized, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Stats: map[string]any{"files_scanned": 1},
		Findings: []domain.Finding{finding},
	})
	require.NoError(t, err)
	require.True(t, finalized, "the only job for this scan just went terminal")

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
	require.Equal(t, "commit a lockfile", list[0].Remediation, "Remediation must round-trip through ListByScan, not just the insert path")
	require.Len(t, list[0].Evidence, 1, "finding_evidence is written by insertFindings but was never read back by ListByScan until this fix")
	require.Equal(t, domain.EvidenceKindManifestExcerpt, list[0].Evidence[0].Kind)
	require.Equal(t, "no lockfile present", list[0].Evidence[0].Value)

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, gotScan.Status, "the scan finalises once its only job is terminal")
	require.NotNil(t, gotScan.FinishedAt)
	require.Equal(t, 1, gotScan.FindingCounts[domain.SeverityMedium])
}

// TestFindingRepo_ListByScan_EvidenceDoesNotLeakBetweenFindings guards the
// batch-fetch in attachEvidence: it maps finding_evidence rows back onto
// findings by finding_id in one query, so a bug there would silently
// attach one finding's evidence to another rather than erroring.
func TestFindingRepo_ListByScan_EvidenceDoesNotLeakBetweenFindings(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, pool)
	scans := repo.NewScanRepo(pool)
	jobs := repo.NewScanJobRepo(pool)
	findings := repo.NewFindingRepo(pool)
	jobResults := repo.NewJobResultRepo(pool)
	seedRule(t, pool, "depscan.secrets.committed-credential")
	seedRule(t, pool, "depscan.hygiene.wildcard-version")

	scan := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}}
	require.NoError(t, scans.Create(ctx, scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(ctx, []domain.ScanJob{job}))

	secretFinding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.secrets.committed-credential",
		Fingerprint: "fp-secret", Title: "hardcoded credential", Description: "d", Severity: domain.SeverityCritical,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeFile, Path: "login/page.tsx", LineStart: 42},
		Remediation: "move to an env var", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: "password: '••••••••'", Redacted: true, LineStart: 42, LineEnd: 42}},
	}
	wildcardFinding := domain.Finding{
		ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, RuleID: "depscan.hygiene.wildcard-version",
		Fingerprint: "fp-wildcard", Title: "wildcard version", Description: "d", Severity: domain.SeverityMedium,
		Confidence: domain.ConfidenceHigh, Location: domain.Location{Type: domain.LocationTypeDependency, Ecosystem: "npm", Package: "left-pad"},
		Remediation: "pin a version", Status: domain.StatusOpen,
		Evidence: []domain.Evidence{{Kind: domain.EvidenceKindManifestExcerpt, Value: `"left-pad": "*"`}},
	}

	_, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{
		JobID: job.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan,
		Status: domain.JobStatusSucceeded, Findings: []domain.Finding{secretFinding, wildcardFinding},
	})
	require.NoError(t, err)

	list, total, err := findings.ListByScan(ctx, scan.ID, orchestrator.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, list, 2)

	byRuleID := map[string]domain.Finding{}
	for _, f := range list {
		byRuleID[f.RuleID] = f
	}

	secret := byRuleID["depscan.secrets.committed-credential"]
	require.Len(t, secret.Evidence, 1)
	require.Equal(t, "password: '••••••••'", secret.Evidence[0].Value)
	require.Equal(t, 42, secret.Evidence[0].LineStart)

	wildcard := byRuleID["depscan.hygiene.wildcard-version"]
	require.Len(t, wildcard.Evidence, 1)
	require.Equal(t, `"left-pad": "*"`, wildcard.Evidence[0].Value)
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

	_, err := jobResults.PersistJobResult(ctx, result)
	require.NoError(t, err)
	// Re-run with a fresh finding ID but the same fingerprint — simulating
	// the same job re-executing (NFR-REL-002: re-runs are idempotent).
	finding.ID = id.New()
	result.Findings = []domain.Finding{finding}
	_, err = jobResults.PersistJobResult(ctx, result)
	require.NoError(t, err)

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

	finalizedA, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobA.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineDepScan, Status: domain.JobStatusSucceeded})
	require.NoError(t, err)
	require.False(t, finalizedA, "jobB is still queued")

	gotScan, err := scans.GetByID(ctx, scan.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, gotScan.Status, "must not finalise while jobB is still queued")

	finalizedB, err := jobResults.PersistJobResult(ctx, orchestrator.JobResult{JobID: jobB.ID, ScanID: scan.ID, ProjectID: projectID, Engine: domain.EngineCodeScan, Status: domain.JobStatusSkipped, SkipReason: "no manifest"})
	require.NoError(t, err)
	require.True(t, finalizedB, "jobB was the last job")

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
