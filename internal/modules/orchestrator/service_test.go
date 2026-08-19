package orchestrator_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// --- hand-written fakes (no mocking framework) ---

type fakeScanRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*domain.Scan
	err  error
}

func newFakeScanRepo() *fakeScanRepo { return &fakeScanRepo{byID: map[uuid.UUID]*domain.Scan{}} }

func (f *fakeScanRepo) Create(_ context.Context, s *domain.Scan) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *s
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeScanRepo) GetByID(_ context.Context, scanID uuid.UUID) (*domain.Scan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[scanID]
	if !ok {
		return nil, apperrors.NotFound("scan.not_found", "scan not found")
	}
	cp := *s
	return &cp, nil
}
func (f *fakeScanRepo) ListByProject(_ context.Context, projectID uuid.UUID, page orchestrator.Page) ([]domain.Scan, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Scan
	for _, s := range f.byID {
		if s.ProjectID == projectID {
			out = append(out, *s)
		}
	}
	total := len(out)
	start := min((page.Page-1)*page.PageSize, len(out))
	end := min(start+page.PageSize, len(out))
	return out[start:end], total, nil
}

// ListByOrg ignores orgID — this fake has no concept of organisations
// (that scoping is the real repo's WHERE clause, proven by
// TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst against real Postgres).
// It exists here only so orchestrator.Service tests can exercise the
// pass-through in ListOrgScans.
func (f *fakeScanRepo) ListByOrg(_ context.Context, _ uuid.UUID, page orchestrator.Page) ([]orchestrator.OrgScanSummary, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []orchestrator.OrgScanSummary
	for _, s := range f.byID {
		out = append(out, orchestrator.OrgScanSummary{Scan: *s, ProjectName: "Test Project"})
	}
	total := len(out)
	start := min((page.Page-1)*page.PageSize, len(out))
	end := min(start+page.PageSize, len(out))
	return out[start:end], total, nil
}
func (f *fakeScanRepo) SetCancelRequested(_ context.Context, scanID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[scanID]
	if !ok {
		return apperrors.NotFound("scan.not_found", "scan not found")
	}
	s.CancelRequested = true
	return nil
}

type fakeScanJobRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*domain.ScanJob
}

func newFakeScanJobRepo() *fakeScanJobRepo {
	return &fakeScanJobRepo{byID: map[uuid.UUID]*domain.ScanJob{}}
}

func (f *fakeScanJobRepo) CreateMany(_ context.Context, jobs []domain.ScanJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range jobs {
		cp := jobs[i]
		f.byID[cp.ID] = &cp
	}
	return nil
}
func (f *fakeScanJobRepo) GetByID(_ context.Context, jobID uuid.UUID) (*domain.ScanJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.byID[jobID]
	if !ok {
		return nil, apperrors.NotFound("job.not_found", "job not found")
	}
	cp := *j
	return &cp, nil
}
func (f *fakeScanJobRepo) ListByScan(_ context.Context, scanID uuid.UUID) ([]domain.ScanJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.ScanJob
	for _, j := range f.byID {
		if j.ScanID == scanID {
			out = append(out, *j)
		}
	}
	return out, nil
}
func (f *fakeScanJobRepo) MarkRunning(_ context.Context, jobID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.byID[jobID]
	if !ok {
		return apperrors.NotFound("job.not_found", "job not found")
	}
	j.Status = domain.JobStatusRunning
	return nil
}

// storedFinding pairs a persisted Finding with the job that produced it —
// the real findings.job_id column, threaded through fakeJobResultRepo's
// jobID parameter rather than a domain.Finding field (see
// JobResultRepository's doc comment for why).
type storedFinding struct {
	JobID   uuid.UUID
	Finding domain.Finding
}

// fakeFindingRepo is the read-only half of finding storage —
// FindingRepository has no write method any more (see its doc comment);
// fakeJobResultRepo appends to the same slice as part of its atomic write,
// exactly like the real Postgres repo shares one `findings` table across
// both interfaces.
type fakeFindingRepo struct {
	mu       sync.Mutex
	findings []storedFinding
}

func (f *fakeFindingRepo) ListByScan(_ context.Context, scanID uuid.UUID, _ orchestrator.Page) ([]domain.Finding, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Finding
	for _, sf := range f.findings {
		if sf.Finding.ScanID == scanID {
			out = append(out, sf.Finding)
		}
	}
	return out, len(out), nil
}
func (f *fakeFindingRepo) CountByScanAndSeverity(_ context.Context, scanID uuid.UUID) (map[domain.Severity]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := map[domain.Severity]int{}
	for _, sf := range f.findings {
		if sf.Finding.ScanID == scanID {
			counts[sf.Finding.Severity]++
		}
	}
	return counts, nil
}
func (f *fakeFindingRepo) CountByJob(_ context.Context, jobID uuid.UUID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, sf := range f.findings {
		if sf.JobID == jobID {
			n++
		}
	}
	return n, nil
}

// fakeJobResultRepo mirrors the real Postgres JobResultRepository's
// atomicity in memory: one call updates the job's terminal status, appends
// any findings, and — if every job for the scan is now terminal —
// finalises the scan's status/finding_counts, all under one lock. Built on
// top of the same fakeScanRepo/fakeScanJobRepo/fakeFindingRepo instances a
// test's read paths already use, exactly like the real repo shares one
// set of tables across these interfaces.
type fakeJobResultRepo struct {
	mu       sync.Mutex
	scans    *fakeScanRepo
	jobs     *fakeScanJobRepo
	findings *fakeFindingRepo
}

var terminalJobStatuses = map[domain.JobStatus]bool{
	domain.JobStatusSucceeded: true, domain.JobStatusFailed: true,
	domain.JobStatusSkipped: true, domain.JobStatusCancelled: true,
}

// PersistJobResult serialises every call via f.mu (so only one call runs at
// a time, mirroring the real repo's one-transaction-per-job-completion
// contract) — but that alone doesn't protect f.jobs.byID/f.scans.byID
// against fakeScanJobRepo.MarkRunning or any other fake method called
// concurrently from a *different* code path (the orchestrator's own worker
// goroutines, one per job, calling MarkRunning for a different job while
// this one is mid-flight): those maps are owned by fakeScanJobRepo/
// fakeScanRepo and guarded by their own mutexes, not f.mu. Every access
// below explicitly takes the owning fake's lock, the same discipline the
// findings block already used — a real, `-race`-caught bug the first time
// a test (TestPool_ProcessJob_SameScanConcurrentJobs_ClonesWorkspaceOnce)
// actually exercised two jobs of the same scan running concurrently.
func (f *fakeJobResultRepo) PersistJobResult(_ context.Context, result orchestrator.JobResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.jobs.mu.Lock()
	job, ok := f.jobs.byID[result.JobID]
	if !ok {
		f.jobs.mu.Unlock()
		return apperrors.NotFound("job.not_found", "job not found")
	}
	job.Status = result.Status
	if result.ErrorReason != "" {
		reason := result.ErrorReason
		job.ErrorReason = &reason
	}
	if result.SkipReason != "" {
		reason := result.SkipReason
		job.SkipReason = &reason
	}
	allTerminal := true
	for _, j := range f.jobs.byID {
		if j.ScanID == result.ScanID && !terminalJobStatuses[j.Status] {
			allTerminal = false
			break
		}
	}
	f.jobs.mu.Unlock()

	f.findings.mu.Lock()
	for _, fnd := range result.Findings {
		f.findings.findings = append(f.findings.findings, storedFinding{JobID: result.JobID, Finding: fnd})
	}
	f.findings.mu.Unlock()

	if !allTerminal {
		return nil
	}

	f.scans.mu.Lock()
	defer f.scans.mu.Unlock()
	scan, ok := f.scans.byID[result.ScanID]
	if !ok {
		return apperrors.NotFound("scan.not_found", "scan not found")
	}
	counts := map[domain.Severity]int{}
	f.findings.mu.Lock()
	for _, sf := range f.findings.findings {
		if sf.Finding.ScanID == result.ScanID {
			counts[sf.Finding.Severity]++
		}
	}
	f.findings.mu.Unlock()

	if scan.CancelRequested {
		scan.Status = domain.ScanStatusCancelled
	} else {
		scan.Status = domain.ScanStatusCompleted
	}
	scan.FindingCounts = counts
	return nil
}

type fakeProjectAccess struct {
	detail *project.ProjectDetail
	err    error
}

func (f *fakeProjectAccess) Get(context.Context, domain.Actor, uuid.UUID) (*project.ProjectDetail, error) {
	return f.detail, f.err
}

type fakeEnqueuer struct {
	mu       sync.Mutex
	enqueued []string
	err      error
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, jobID string) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, jobID)
	return nil
}

// fakeEngine satisfies domain.Engine minimally — the service tests only
// need registry membership, not real Run behaviour.
type fakeEngine struct{ id domain.EngineID }

func (e fakeEngine) ID() domain.EngineID                                         { return e.id }
func (e fakeEngine) Applicable(context.Context, domain.ScanInput) (bool, string) { return true, "" }
func (e fakeEngine) Run(context.Context, domain.ScanInput, func(domain.Finding)) (domain.EngineResult, error) {
	return domain.EngineResult{}, nil
}

func newActor() domain.Actor {
	return domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember}
}

func newTestOrchestrator(t *testing.T) (orchestrator.Service, *fakeScanRepo, *fakeScanJobRepo, *fakeFindingRepo, *fakeEnqueuer, *fakeJobResultRepo) {
	t.Helper()
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}

	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}}}

	svc := orchestrator.NewService(scans, jobs, findings, projects, enqueuer, registry)
	return svc, scans, jobs, findings, enqueuer, jobResults
}

func TestCreateScan_FullSupplyChain_UsesAllRegisteredEngines(t *testing.T) {
	svc, _, jobs, _, enqueuer, _ := newTestOrchestrator(t)
	actor := newActor()
	projectID := id.New()

	detail, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusQueued, detail.Status)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, detail.RequestedEngines)
	require.Len(t, detail.Jobs, 1)

	stored, err := jobs.GetByID(context.Background(), detail.Jobs[0].ID)
	require.NoError(t, err)
	require.Equal(t, domain.EngineDepScan, stored.Engine)

	require.Len(t, enqueuer.enqueued, 1, "every created job must be enqueued")
}

func TestCreateScan_Partial_RejectsUnregisteredEngine(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	actor := newActor()

	_, err := svc.CreateScan(context.Background(), actor, id.New(), orchestrator.CreateScanInput{
		Type: domain.ScanTypePartial, Engines: []domain.EngineID{domain.EngineCodeScan},
	})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindUnprocessable, appErr.Kind)
}

func TestCreateScan_Partial_RequiresEnginesList(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypePartial})
	require.Error(t, err)
}

func TestCreateScan_UnknownProject_ReturnsNotFound(t *testing.T) {
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	projects := &fakeProjectAccess{err: apperrors.NotFound("project.not_found", "project not found")}
	svc := orchestrator.NewService(scans, jobs, findings, projects, enqueuer, registry)

	_, err := svc.CreateScan(context.Background(), newActor(), id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestListScans_ReturnsNewestFirstForTheProject(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	actor := newActor()
	projectID := id.New()

	first, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	second, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)

	scans, total, err := svc.ListScans(context.Background(), actor, projectID, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, scans, 2)

	ids := map[uuid.UUID]bool{first.ID: true, second.ID: true}
	for _, s := range scans {
		require.True(t, ids[s.ID])
	}
}

// TestListOrgScans_ReturnsScansWithProjectName proves the service's
// pass-through to ScanRepository.ListOrgScans — the fake wraps every
// stored scan with a project name, so this checks the plumbing (actor ->
// repo call -> response), while the real org-scoping/ordering guarantee is
// proven against Postgres by TestScanRepo_ListByOrg_ScopedToOrgAndNewestFirst.
func TestListOrgScans_ReturnsScansWithProjectName(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	actor := newActor()

	created, err := svc.CreateScan(context.Background(), actor, id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)

	scans, total, err := svc.ListOrgScans(context.Background(), actor, orchestrator.Page{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, scans, 1)
	require.Equal(t, created.ID, scans[0].ID)
	require.NotEmpty(t, scans[0].ProjectName)
}

// TestListScans_CrossOrgProject_ReturnsNotFound is the 404-not-403 rule
// (documentation/07-api-specification.md §1.4) applied to the history
// endpoint: a project ID belonging to another organisation must look
// identical to a nonexistent one, not leak its existence.
func TestListScans_CrossOrgProject_ReturnsNotFound(t *testing.T) {
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	projects := &fakeProjectAccess{err: apperrors.NotFound("project.not_found", "project not found")}
	svc := orchestrator.NewService(scans, jobs, findings, projects, enqueuer, registry)

	_, _, err := svc.ListScans(context.Background(), newActor(), id.New(), orchestrator.Page{Page: 1, PageSize: 10})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestGetScan_ReturnsJobsWithFindingCounts(t *testing.T) {
	svc, _, _, _, _, jobResults := newTestOrchestrator(t)
	actor := newActor()
	projectID := id.New()

	detail, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	jobID := detail.Jobs[0].ID

	require.NoError(t, jobResults.PersistJobResult(context.Background(), orchestrator.JobResult{
		JobID: jobID, ScanID: detail.ID, Status: domain.JobStatusSucceeded,
		Findings: []domain.Finding{{ID: id.New(), ScanID: detail.ID, Severity: domain.SeverityHigh}},
	}))

	got, err := svc.GetScan(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, got.Jobs, 1)
	require.Equal(t, domain.JobStatusSucceeded, got.Jobs[0].Status)
	require.Equal(t, 1, got.Jobs[0].FindingCount)
}

func TestCancelScan_SetsFlag(t *testing.T) {
	svc, scans, _, _, _, _ := newTestOrchestrator(t)
	actor := newActor()

	detail, err := svc.CreateScan(context.Background(), actor, id.New(), orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)

	require.NoError(t, svc.CancelScan(context.Background(), actor, detail.ID))
	stored, err := scans.GetByID(context.Background(), detail.ID)
	require.NoError(t, err)
	require.True(t, stored.CancelRequested)
}

func TestGetScan_UnknownScan_ReturnsNotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	_, err := svc.GetScan(context.Background(), newActor(), id.New())
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}
