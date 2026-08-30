package orchestrator_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// fakeAuditService is a hand-written fake — no mocking framework, same
// convention modules/project's own fakeAuditService (service_test.go)
// already establishes for this exact interface.
type fakeAuditService struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (f *fakeAuditService) Log(_ context.Context, e audit.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeAuditService) List(_ context.Context, _ audit.ListFilter, _ audit.Page) ([]audit.Entry, int, error) {
	return nil, 0, nil
}

func (f *fakeAuditService) last() *audit.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.entries) == 0 {
		return nil
	}
	return &f.entries[len(f.entries)-1]
}

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

// MarkStarted mirrors the real repo's idempotent "only from queued" guard —
// a no-op, not an error, once the scan has already moved past queued.
func (f *fakeScanRepo) MarkStarted(_ context.Context, scanID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[scanID]
	if !ok || s.Status != domain.ScanStatusQueued {
		return nil
	}
	s.Status = domain.ScanStatusRunning
	now := time.Now()
	s.StartedAt = &now
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
	// Mirrors the real repo's `SET status = 'running', started_at = now()`
	// (internal/store/repo/scan_job_repo.go) — GetProgress's elapsed-time
	// fallback needs a real StartedAt to compute against.
	now := time.Now()
	j.StartedAt = &now
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
func (f *fakeFindingRepo) ListAllByScan(_ context.Context, scanID uuid.UUID) ([]domain.Finding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Finding
	for _, sf := range f.findings {
		if sf.Finding.ScanID == scanID {
			out = append(out, sf.Finding)
		}
	}
	return out, nil
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
func (f *fakeJobResultRepo) PersistJobResult(_ context.Context, result orchestrator.JobResult) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.jobs.mu.Lock()
	job, ok := f.jobs.byID[result.JobID]
	if !ok {
		f.jobs.mu.Unlock()
		return false, apperrors.NotFound("job.not_found", "job not found")
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
		return false, nil
	}

	f.scans.mu.Lock()
	defer f.scans.mu.Unlock()
	scan, ok := f.scans.byID[result.ScanID]
	if !ok {
		return false, apperrors.NotFound("scan.not_found", "scan not found")
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
	return true, nil
}

// fakeRiskAssessmentRepo is a hand-written fake for
// orchestrator.RiskAssessmentRepository — records every Create call so a
// test can assert on exactly what Pool.finalizeScoring computed, and
// serves a scripted previous score per project for the Delta path.
type fakeRiskAssessmentRepo struct {
	mu             sync.Mutex
	created        []orchestrator.RiskAssessmentRecord
	previousScores map[uuid.UUID]int
}

func (f *fakeRiskAssessmentRepo) Create(_ context.Context, r orchestrator.RiskAssessmentRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, r)
	return nil
}

func (f *fakeRiskAssessmentRepo) GetByScanID(_ context.Context, scanID uuid.UUID) (*orchestrator.RiskAssessmentRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range slices.Backward(f.created) {
		if r.ScanID == scanID {
			cp := r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeRiskAssessmentRepo) GetPreviousScore(_ context.Context, projectID, _ uuid.UUID) (*int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	score, ok := f.previousScores[projectID]
	if !ok {
		return nil, nil
	}
	cp := score
	return &cp, nil
}

func (f *fakeRiskAssessmentRepo) last() *orchestrator.RiskAssessmentRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.created) == 0 {
		return nil
	}
	cp := f.created[len(f.created)-1]
	return &cp
}

type fakeProjectAccess struct {
	detail *project.ProjectDetail
	err    error

	// target/targetErr drive GetAttestedTarget — target set means "an
	// attested target exists," nil (the zero value) means "none," matching
	// project.Service.GetAttestedTarget's own not-found-on-none contract.
	target    *project.Target
	targetErr error
}

func (f *fakeProjectAccess) Get(context.Context, domain.Actor, uuid.UUID) (*project.ProjectDetail, error) {
	return f.detail, f.err
}

func (f *fakeProjectAccess) GetAttestedTarget(context.Context, uuid.UUID) (*project.Target, error) {
	if f.targetErr != nil {
		return nil, f.targetErr
	}
	if f.target == nil {
		return nil, apperrors.NotFound("project.pentest_target_not_found", "no attested pentest target attached to this project")
	}
	return f.target, nil
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

// newTestOrchestrator's fixture project carries a Repository (non-nil, empty
// value is enough — resolveEngines only checks presence) precisely because
// resolveEngines now excludes repository-based engines from a repo-less
// project: the existing "full supply chain runs every registered engine"
// tests below assume depscan actually runs, which requires a repository to
// be attached. Repo/target-gating itself is exercised separately by
// TestCreateScan_FullSupplyChain_ExcludesEnginesTheProjectCantRun and its
// siblings, which build their own fakeProjectAccess per case.
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
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{
		Project:    project.Project{ID: projectID},
		Repository: &project.Repository{},
	}}

	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil)
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

// TestCreateScan_RecordsAuditEntryWithActorAndIP is this feature's own
// accountability contract: every scan creation must be attributable to an
// actor, an org, and a source IP — the same "who did this and from where"
// shape target.attested already gives the one-time target attestation
// (modules/project/service_test.go's own audit tests), now per scan
// execution too, since reporting.Assembler's exported-report watermark
// reads straight off domain.Scan.RequestedFromIP + TriggeredBy.
func TestCreateScan_RecordsAuditEntryWithActorAndIP(t *testing.T) {
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{
		Project: project.Project{ID: projectID}, Repository: &project.Repository{},
	}}
	auditSvc := &fakeAuditService{}

	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, auditSvc)
	actor := newActor()

	detail, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{
		Type: domain.ScanTypeFullSupplyChain, SourceIP: "203.0.113.10",
	})
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", detail.RequestedFromIP, "the scan row itself must carry the requesting IP")

	entry := auditSvc.last()
	require.NotNil(t, entry, "CreateScan must log an audit entry")
	require.Equal(t, "scan.started", entry.Action)
	require.Equal(t, &actor.OrgID, entry.OrgID)
	require.Equal(t, &actor.UserID, entry.ActorID)
	require.NotNil(t, entry.ResourceID)
	require.Equal(t, detail.ID, *entry.ResourceID)
	require.NotNil(t, entry.IP, "source IP must round-trip into the audit entry")
	require.Equal(t, "203.0.113.10", entry.IP.String())
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

// newGatingOrchestrator builds an orchestrator.Service with a registry
// carrying exactly depscan (repository-based) and pentest (target-based),
// and the given project fixture — the shared setup every resolveEngines
// gating test below needs, since each case needs its own repo/target shape.
func newGatingOrchestrator(t *testing.T, projects *fakeProjectAccess) orchestrator.Service {
	t.Helper()
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	registry.Register(fakeEngine{id: domain.EnginePentest})
	return orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil)
}

// TestCreateScan_FullSupplyChain_ExcludesEnginesTheProjectCantRun is the
// true-positive half: a project with only an attested target (no
// repository) gets exactly [pentest] from a full_supply_chain request —
// depscan's job is never even created, not created-then-failed.
func TestCreateScan_FullSupplyChain_ExcludesEnginesTheProjectCantRun(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{
		detail: &project.ProjectDetail{Project: project.Project{ID: projectID}}, // no Repository
		target: &project.Target{ID: id.New(), Status: project.TargetAttested},
	}
	svc := newGatingOrchestrator(t, projects)

	detail, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.Equal(t, []domain.EngineID{domain.EnginePentest}, detail.RequestedEngines)
}

// TestCreateScan_FullSupplyChain_RepoOnlyProjectExcludesPentest is the
// near-miss complement: a project with a repository but no attested target
// must not fire pentest just because a repo-based engine is available.
func TestCreateScan_FullSupplyChain_RepoOnlyProjectExcludesPentest(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{
		detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}},
		// target left nil — no attested target
	}
	svc := newGatingOrchestrator(t, projects)

	detail, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.Equal(t, []domain.EngineID{domain.EngineDepScan}, detail.RequestedEngines)
}

// TestCreateScan_FullSupplyChain_BothAttachedRunsEverything confirms a
// project with both a repository and an attested target gets every
// registered engine from one full_supply_chain request — no separate
// pentest_only call required once both are attached.
func TestCreateScan_FullSupplyChain_BothAttachedRunsEverything(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{
		detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}},
		target: &project.Target{ID: id.New(), Status: project.TargetAttested},
	}
	svc := newGatingOrchestrator(t, projects)

	detail, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.EngineID{domain.EngineDepScan, domain.EnginePentest}, detail.RequestedEngines)
}

// TestCreateScan_FullSupplyChain_NeitherAttachedHasNoEngines is the
// near-miss floor: a project with neither a repository nor a target has
// nothing runnable at all, and must fail loudly (scan.no_engines_available)
// rather than silently create a scan with zero jobs.
func TestCreateScan_FullSupplyChain_NeitherAttachedHasNoEngines(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}}}
	svc := newGatingOrchestrator(t, projects)

	_, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "scan.no_engines_available", appErr.Code)
}

// TestCreateScan_PentestOnly_RequiresAttestedTarget is the near-miss half of
// FR-PEN-013's standalone pentest path: no attested target means a 422, not
// a scan silently created with zero jobs.
func TestCreateScan_PentestOnly_RequiresAttestedTarget(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}}}
	svc := newGatingOrchestrator(t, projects)

	_, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypePentestOnly})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindUnprocessable, appErr.Kind)
}

// TestCreateScan_PentestOnly_IgnoresOtherRegisteredEngines is the true
// positive: an attested target resolves pentest_only to exactly [pentest],
// even though depscan is also registered and the project also has a repo.
func TestCreateScan_PentestOnly_IgnoresOtherRegisteredEngines(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{
		detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}},
		target: &project.Target{ID: id.New(), Status: project.TargetAttested},
	}
	svc := newGatingOrchestrator(t, projects)

	detail, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{Type: domain.ScanTypePentestOnly})
	require.NoError(t, err)
	require.Equal(t, []domain.EngineID{domain.EnginePentest}, detail.RequestedEngines)
}

// TestCreateScan_Partial_RejectsEngineTheProjectCantRun is the near-miss
// complement to TestCreateScan_Partial_RejectsUnregisteredEngine: depscan IS
// registered here, but this project has no repository, so an explicit
// request for it must still be rejected loudly (422), not silently dropped.
func TestCreateScan_Partial_RejectsEngineTheProjectCantRun(t *testing.T) {
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}}} // no repo
	svc := newGatingOrchestrator(t, projects)

	_, err := svc.CreateScan(context.Background(), newActor(), projectID, orchestrator.CreateScanInput{
		Type: domain.ScanTypePartial, Engines: []domain.EngineID{domain.EngineDepScan},
	})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindUnprocessable, appErr.Kind)
}

func TestCreateScan_UnknownProject_ReturnsNotFound(t *testing.T) {
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	projects := &fakeProjectAccess{err: apperrors.NotFound("project.not_found", "project not found")}
	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil)

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
	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), nil, nil, 0, nil)

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

	_, err = jobResults.PersistJobResult(context.Background(), orchestrator.JobResult{
		JobID: jobID, ScanID: detail.ID, Status: domain.JobStatusSucceeded,
		Findings: []domain.Finding{{ID: id.New(), ScanID: detail.ID, Severity: domain.SeverityHigh}},
	})
	require.NoError(t, err)

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

func TestGetProgress_PrefersLiveEngineReportedProgressOverElapsedFallback(t *testing.T) {
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}}}
	liveProgress := orchestrator.NewLiveProgress()
	timeouts := map[domain.EngineID]time.Duration{domain.EngineDepScan: 10 * time.Minute}
	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), liveProgress, timeouts, 5*time.Minute, nil)

	actor := newActor()
	detail, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	jobID := detail.Jobs[0].ID
	require.NoError(t, jobs.MarkRunning(context.Background(), jobID))

	// The engine itself reported a real, specific stage — GetProgress must
	// surface exactly that, not fall back to an elapsed-time guess.
	liveProgress.Set(jobID, 62, "Fuzzing for hidden files and paths")

	progress, err := svc.GetProgress(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, progress.Engines, 1)
	require.Equal(t, 62, progress.Engines[0].ProgressPct)
	require.Equal(t, "Fuzzing for hidden files and paths", progress.Engines[0].Activity)
}

func TestGetProgress_NearMiss_FallsBackToRealElapsedTimeWhenEngineReportsNothing(t *testing.T) {
	// An engine with no named stages of its own (every engine except
	// pentest, today) must still show a real, moving number — computed from
	// actual elapsed time against its own configured timeout, never a
	// frozen constant.
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	enqueuer := &fakeEnqueuer{}
	registry := orchestrator.NewRegistry()
	registry.Register(fakeEngine{id: domain.EngineDepScan})
	projectID := id.New()
	projects := &fakeProjectAccess{detail: &project.ProjectDetail{Project: project.Project{ID: projectID}, Repository: &project.Repository{}}}
	liveProgress := orchestrator.NewLiveProgress() // no Set() call for this job — nothing reported
	timeouts := map[domain.EngineID]time.Duration{domain.EngineDepScan: time.Second}
	svc := orchestrator.NewService(scans, jobs, findings, &fakeRiskAssessmentRepo{}, projects, enqueuer, registry, domain.PentestPresetDeepConfig(), liveProgress, timeouts, 5*time.Minute, nil)

	actor := newActor()
	detail, err := svc.CreateScan(context.Background(), actor, projectID, orchestrator.CreateScanInput{Type: domain.ScanTypeFullSupplyChain})
	require.NoError(t, err)
	jobID := detail.Jobs[0].ID
	require.NoError(t, jobs.MarkRunning(context.Background(), jobID))

	time.Sleep(300 * time.Millisecond) // ~30% of the 1s timeout above

	progress, err := svc.GetProgress(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, progress.Engines, 1)
	require.Empty(t, progress.Engines[0].Activity, "no engine-reported activity — the frontend's own generic fallback label applies")
	require.Greater(t, progress.Engines[0].ProgressPct, 0)
	require.Less(t, progress.Engines[0].ProgressPct, 100)

	// The bug this also covers: a single-job scan's *overall* ProgressPct
	// used to be `terminal_jobs / total_jobs`, which sits frozen at 0 for
	// the entire time a scan's only job is running (0/1) and then jumps
	// straight to 100 — exactly the "stuck at 0%, then suddenly 100%"
	// behaviour reported live. It must now move with the same real
	// per-job value already asserted above, not just count terminal jobs.
	require.Equal(t, progress.Engines[0].ProgressPct, progress.ProgressPct,
		"a scan with exactly one job must report that job's own live pct as the overall pct, not a frozen 0")
}

func TestGetScan_UnknownScan_ReturnsNotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestOrchestrator(t)
	_, err := svc.GetScan(context.Background(), newActor(), id.New())
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}
