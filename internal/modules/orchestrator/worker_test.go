package orchestrator_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeQueue is an in-memory stand-in for adapters/queue.JobQueue — one
// Claim call returns the next queued ID and then blocks (context-aware)
// once drained, matching the real queue's blocking-claim contract closely
// enough for these tests.
type fakeQueue struct {
	mu      sync.Mutex
	pending []string
	acked   []string
}

func (q *fakeQueue) claim(ctx context.Context, _ time.Duration) (string, error) {
	q.mu.Lock()
	if len(q.pending) > 0 {
		next := q.pending[0]
		q.pending = q.pending[1:]
		q.mu.Unlock()
		return next, nil
	}
	q.mu.Unlock()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(20 * time.Millisecond):
		return "", nil
	}
}

func (q *fakeQueue) ack(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.acked = append(q.acked, jobID)
	return nil
}

type fakeCloner struct {
	err   error
	delay time.Duration
	// populate, when set, runs once destDir exists (real prepareWorkspace
	// has already os.MkdirTemp'd it) but before ShallowClone returns — a
	// stand-in for whatever content a real `git clone` would have left
	// behind, so tests can plant filesystem shapes (e.g. a symlink cycle)
	// that hardenWorkspace then runs against for real.
	populate func(destDir string) error

	mu       sync.Mutex
	calls    int
	destDirs []string
}

func (c *fakeCloner) ShallowClone(ctx context.Context, _, _, destDir string) error {
	c.mu.Lock()
	c.calls++
	c.destDirs = append(c.destDirs, destDir)
	c.mu.Unlock()

	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.err != nil {
		return c.err
	}
	if c.populate != nil {
		return c.populate(destDir)
	}
	return nil
}

func (c *fakeCloner) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type fakeCloneInfo struct {
	mu                    sync.Mutex
	token                 string
	noRepository          bool
	invalidatedProjectIDs []uuid.UUID
	invalidatedReasons    []string
}

func (f *fakeCloneInfo) GetCloneInfo(context.Context, uuid.UUID) (string, string, string, error) {
	if f.noRepository {
		return "", "", "", apperrors.NotFound("project.repository_not_found", "no repository attached to this project")
	}
	return "https://github.com/acme/example", "main", f.token, nil
}

func (f *fakeCloneInfo) MarkCredentialInvalid(_ context.Context, projectID uuid.UUID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidatedProjectIDs = append(f.invalidatedProjectIDs, projectID)
	f.invalidatedReasons = append(f.invalidatedReasons, reason)
	return nil
}

func (f *fakeCloneInfo) invalidatedCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.invalidatedProjectIDs)
}

// scriptedEngine lets each test dictate exactly what Applicable/Run do —
// including panicking, the case runEngine's recover exists for.
type scriptedEngine struct {
	id         domain.EngineID
	applicable bool
	reason     string
	findings   []domain.Finding
	runErr     error
	panicOnRun bool
}

func (e *scriptedEngine) ID() domain.EngineID { return e.id }
func (e *scriptedEngine) Applicable(context.Context, domain.ScanInput) (bool, string) {
	return e.applicable, e.reason
}
func (e *scriptedEngine) Run(_ context.Context, _ domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	if e.panicOnRun {
		panic("simulated engine panic")
	}
	for _, f := range e.findings {
		emit(f)
	}
	return domain.EngineResult{RulesEvaluated: 1}, e.runErr
}

// recordingEngine lets a test observe the real domain.ScanInput a job ran
// with (in particular WorkspaceDir's actual on-disk contents at the moment
// the engine sees it) — scriptedEngine's fixed applicable/findings/runErr
// script has no hook for that.
type recordingEngine struct {
	id    domain.EngineID
	check func(in domain.ScanInput)
}

func (e *recordingEngine) ID() domain.EngineID { return e.id }
func (e *recordingEngine) Applicable(context.Context, domain.ScanInput) (bool, string) {
	return true, ""
}
func (e *recordingEngine) Run(_ context.Context, in domain.ScanInput, _ func(domain.Finding)) (domain.EngineResult, error) {
	if e.check != nil {
		e.check(in)
	}
	return domain.EngineResult{RulesEvaluated: 1}, nil
}

func newTestPool(t *testing.T, engine domain.Engine, cloner *fakeCloner) (*orchestrator.Pool, *fakeScanRepo, *fakeScanJobRepo, *fakeFindingRepo, *fakeQueue) {
	t.Helper()
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engine)
	q := &fakeQueue{}

	pool := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects: &fakeCloneInfo{}, Cloner: cloner,
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}
	return pool, scans, jobs, findings, q
}

func seedScanAndJob(t *testing.T, scans *fakeScanRepo, jobs *fakeScanJobRepo, engineID domain.EngineID) (scanID, jobID uuid.UUID) {
	t.Helper()
	scan := &domain.Scan{ID: id.New(), ProjectID: id.New(), Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusRunning}
	require.NoError(t, scans.Create(context.Background(), scan))
	job := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: engineID, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(context.Background(), []domain.ScanJob{job}))
	return scan.ID, job.ID
}

func TestPool_ProcessJob_Success_PersistsFindingsAndMarksSucceeded(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true, findings: []domain.Finding{{ID: id.New(), RuleID: "depscan.hygiene.no-lockfile", Severity: domain.SeverityMedium}}}
	pool, scans, jobs, findings, q := newTestPool(t, engine, &fakeCloner{})
	scanID, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, job.Status)
	require.Len(t, findings.findings, 1)

	scan, err := scans.GetByID(context.Background(), scanID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCompleted, scan.Status, "the scan finalises once its only job is terminal")
}

func TestPool_ProcessJob_NotApplicable_MarksSkipped(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: false, reason: "no manifest found"}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{})
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSkipped, job.Status)
	require.NotNil(t, job.SkipReason)
	require.Equal(t, "no manifest found", *job.SkipReason)
}

func TestPool_ProcessJob_EnginePanics_MarksFailedNotCrashesWorker(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true, panicOnRun: true}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{})
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	require.NotPanics(t, func() { pool.Start(ctx) }, "a panicking engine must never take down the worker (NFR-REL-001)")

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusFailed, job.Status)
}

func TestPool_ProcessJob_CloneFails_MarksFailed(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{err: errors.New("connection refused")})
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusFailed, job.Status)
	require.NotNil(t, job.ErrorReason)
	require.Equal(t, "workspace_unavailable", *job.ErrorReason)
}

// TestPool_ProcessJob_CloneUnauthorized_FlagsCredentialInvalid is the
// true-positive half of the credential-health table: a clone rejected with
// github.ErrCloneUnauthorized (401/403 — the stored PAT is bad) must both
// persist the specific "credential_invalid" job reason, distinct from a
// generic clone failure, and call CloneInfoProvider.MarkCredentialInvalid so
// the project carries that signal even after this one scan.
func TestPool_ProcessJob_CloneUnauthorized_FlagsCredentialInvalid(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engine)
	q := &fakeQueue{}
	projects := &fakeCloneInfo{token: "ghp_expiredtoken"}

	pool := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects:      projects,
		Cloner:        &fakeCloner{err: github.ErrCloneUnauthorized},
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusFailed, job.Status)
	require.NotNil(t, job.ErrorReason)
	require.Equal(t, "credential_invalid", *job.ErrorReason)
	require.Equal(t, 1, projects.invalidatedCalls(), "MarkCredentialInvalid must be called exactly once")
}

// TestPool_ProcessJob_CloneUnauthorized_NoTokenDoesNotFlagCredential is the
// near-miss: a 401/403 with no credential attached at all (token == "") is
// "no credential was ever configured," not "a credential went bad" — that
// state is already surfaced at attach time (project.credential_required),
// so this must not call MarkCredentialInvalid or use the credential_invalid
// reason.
func TestPool_ProcessJob_CloneUnauthorized_NoTokenDoesNotFlagCredential(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engine)
	q := &fakeQueue{}
	projects := &fakeCloneInfo{token: ""}

	pool := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects:      projects,
		Cloner:        &fakeCloner{err: github.ErrCloneUnauthorized},
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusFailed, job.Status)
	require.NotNil(t, job.ErrorReason)
	require.Equal(t, "workspace_unavailable", *job.ErrorReason)
	require.Equal(t, 0, projects.invalidatedCalls(), "no credential was ever attached, so nothing should be flagged invalid")
}

// TestPool_ProcessJob_DocReview_NoRepositoryAttached_RunsAnyway is
// docreview's own carve-out from TestPool_ProcessJob_CloneFails_MarksFailed:
// unlike every other engine, it can review uploaded documents alone with no
// repository ever attached to the project at all
// (documentation/05-module-specifications.md §11) — GetCloneInfo's
// "project.repository_not_found" must not fail this job the way any other
// workspace-prep failure correctly does; it should just mean "nothing to add
// from a repo checkout," leaving WorkspaceDir empty rather than failing.
func TestPool_ProcessJob_DocReview_NoRepositoryAttached_RunsAnyway(t *testing.T) {
	var seenWorkspaceDir string
	ranEngine := false
	engine := &recordingEngine{id: domain.EngineDocReview, check: func(in domain.ScanInput) {
		ranEngine = true
		seenWorkspaceDir = in.WorkspaceDir
	}}
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engine)
	q := &fakeQueue{}

	pool := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects:      &fakeCloneInfo{noRepository: true},
		Cloner:        &fakeCloner{},
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDocReview)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, job.Status)
	require.True(t, ranEngine, "engine.Run should have been called despite no repository")
	require.Empty(t, seenWorkspaceDir)
}

// TestPool_ProcessJob_OtherEngine_NoRepositoryAttached_StillFails is the
// near-miss: the carve-out above is docreview-specific — every other engine
// still genuinely needs a repository, so "no repository attached" must keep
// failing them with workspace_unavailable exactly as before.
func TestPool_ProcessJob_OtherEngine_NoRepositoryAttached_StillFails(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engine)
	q := &fakeQueue{}

	pool := &orchestrator.Pool{
		Size: 1, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects:      &fakeCloneInfo{noRepository: true},
		Cloner:        &fakeCloner{},
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusFailed, job.Status)
	require.NotNil(t, job.ErrorReason)
	require.Equal(t, "workspace_unavailable", *job.ErrorReason)
}

func TestPool_ProcessJob_CancelledScan_MarksCancelledWithoutRunningEngine(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	pool, scans, jobs, findings, q := newTestPool(t, engine, &fakeCloner{})
	scanID, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	require.NoError(t, scans.SetCancelRequested(context.Background(), scanID))
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	require.Empty(t, findings.findings, "a cancelled scan's job must never reach the engine, so it emits nothing")
	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusCancelled, job.Status)

	scan, err := scans.GetByID(context.Background(), scanID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCancelled, scan.Status)
}

// TestPool_ProcessJob_SameScanConcurrentJobs_ClonesWorkspaceOnce is the
// true-positive half of the workspace-sharing table: documentation/05-module-specifications.md
// §5's pipeline diagram draws one "workspace prep" node feeding every
// repository engine, not one clone per engine. Two workers claiming two
// jobs from the *same* scan concurrently (Size: 2, both jobs pending
// up-front, a real clone delay so the two claims actually overlap) must
// still only clone once — the second job reuses the first's directory
// instead of racing it to clone the same repository a second time.
func TestPool_ProcessJob_SameScanConcurrentJobs_ClonesWorkspaceOnce(t *testing.T) {
	engineA := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	engineB := &scriptedEngine{id: domain.EngineCodeScan, applicable: true}

	scans := newFakeScanRepo()
	jobs := newFakeScanJobRepo()
	findings := &fakeFindingRepo{}
	jobResults := &fakeJobResultRepo{scans: scans, jobs: jobs, findings: findings}
	registry := orchestrator.NewRegistry()
	registry.Register(engineA)
	registry.Register(engineB)
	q := &fakeQueue{}
	cloner := &fakeCloner{delay: 100 * time.Millisecond}

	pool := &orchestrator.Pool{
		Size: 2, Queue: orchestrator.NewJobQueueClaimer(q.claim, q.ack),
		Registry: registry, Scans: scans, Jobs: jobs, JobResults: jobResults,
		Projects: &fakeCloneInfo{}, Cloner: cloner,
		WorkspaceRoot: t.TempDir(), DefaultTimeout: 5 * time.Second,
		Log: discardLogger(),
	}

	scan := &domain.Scan{
		ID: id.New(), ProjectID: id.New(), Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusRunning,
		RequestedEngines: []domain.EngineID{domain.EngineDepScan, domain.EngineCodeScan},
	}
	require.NoError(t, scans.Create(context.Background(), scan))
	jobA := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineDepScan, Status: domain.JobStatusQueued}
	jobB := domain.ScanJob{ID: id.New(), ScanID: scan.ID, Engine: domain.EngineCodeScan, Status: domain.JobStatusQueued}
	require.NoError(t, jobs.CreateMany(context.Background(), []domain.ScanJob{jobA, jobB}))
	q.pending = []string{jobA.ID.String(), jobB.ID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pool.Start(ctx)

	require.Equal(t, 1, cloner.callCount(), "two jobs from the same scan must share one clone, not clone once per engine")

	gotA, err := jobs.GetByID(context.Background(), jobA.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, gotA.Status)
	gotB, err := jobs.GetByID(context.Background(), jobB.ID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, gotB.Status)
}

// TestPool_ProcessJob_DifferentScans_CloneIndependently is the near-miss:
// two unrelated scans must not share a workspace just because they're
// processed by the same pool — each still gets its own clone.
func TestPool_ProcessJob_DifferentScans_CloneIndependently(t *testing.T) {
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{})
	cloner := pool.Cloner.(*fakeCloner)

	_, jobID1 := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	_, jobID2 := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID1.String(), jobID2.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	require.Equal(t, 2, cloner.callCount(), "two different scans must each get their own clone")
}

// requireSymlinkSupport skips a test on a platform/environment that can't
// create symlinks at all — notably Windows without Developer Mode or admin
// rights (the sandbox this suite is often run in locally). CI (Linux) and
// the actual runtime container both support symlinks unconditionally, which
// is what these tests exist to guard; skipping here only narrows *where*
// the guarantee is checked, not what it claims.
func requireSymlinkSupport(t *testing.T) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}
}

// TestPool_ProcessJob_SymlinkCycleInClone_DoesNotFailTheJob reproduces the
// real failure a live scan against github.com/madhuakula/kubernetes-goat
// hit: a symlink cycle inside the cloned repository (its
// scenarios/metadata-db content) made the post-clone permission pass fail
// outright with ELOOP ("too many levels of symbolic links"), because
// os.Chmod — unlike filepath.WalkDir's own traversal — follows symlinks.
func TestPool_ProcessJob_SymlinkCycleInClone_DoesNotFailTheJob(t *testing.T) {
	requireSymlinkSupport(t)
	var sawWorkspaceDir string
	engine := &recordingEngine{id: domain.EngineDepScan, check: func(in domain.ScanInput) {
		sawWorkspaceDir = in.WorkspaceDir
	}}
	cloner := &fakeCloner{populate: func(destDir string) error {
		cycle := filepath.Join(destDir, "cycle")
		return os.Symlink(cycle, cycle) // points at itself
	}}
	pool, scans, jobs, _, q := newTestPool(t, engine, cloner)
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, job.Status, "a symlink cycle inside the cloned repo must not fail the whole job")
	require.NotEmpty(t, sawWorkspaceDir, "the engine should have run with a real workspace directory")
}

// TestPool_ProcessJob_SymlinkEscapingWorkspaceRoot_IsRemoved is the
// true-positive/near-miss pair for documentation/05-module-specifications.md
// §5's "Symlinks that escape the workspace root are removed before engines
// run (path-traversal defence)" rule — documented but, until this fix, never
// actually implemented. A symlink resolving outside the workspace root must
// be gone by the time an engine runs; one resolving inside it must survive.
func TestPool_ProcessJob_SymlinkEscapingWorkspaceRoot_IsRemoved(t *testing.T) {
	requireSymlinkSupport(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o600))

	var sawEscaping, sawInternal bool
	engine := &recordingEngine{id: domain.EngineDepScan, check: func(in domain.ScanInput) {
		_, errEsc := os.Lstat(filepath.Join(in.WorkspaceDir, "escape"))
		sawEscaping = errEsc == nil
		_, errInt := os.Lstat(filepath.Join(in.WorkspaceDir, "internal"))
		sawInternal = errInt == nil
	}}
	cloner := &fakeCloner{populate: func(destDir string) error {
		if err := os.WriteFile(filepath.Join(destDir, "real.txt"), []byte("hi"), 0o600); err != nil {
			return err
		}
		if err := os.Symlink(outsideFile, filepath.Join(destDir, "escape")); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(destDir, "real.txt"), filepath.Join(destDir, "internal"))
	}}
	pool, scans, jobs, _, q := newTestPool(t, engine, cloner)
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	job, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, job.Status)
	require.False(t, sawEscaping, "a symlink resolving outside the workspace root must be removed before an engine ever sees it")
	require.True(t, sawInternal, "a symlink resolving inside the workspace root must be left alone")
}
