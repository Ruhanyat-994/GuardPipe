package orchestrator_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
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
	err error
}

func (c *fakeCloner) ShallowClone(context.Context, string, string, string) error {
	return c.err
}

type fakeCloneInfo struct{}

func (fakeCloneInfo) GetCloneInfo(context.Context, uuid.UUID) (string, string, string, error) {
	return "https://github.com/acme/example", "main", "", nil
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
		Projects: fakeCloneInfo{}, Cloner: cloner,
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

func TestPool_ProcessJob_CancelledScan_MarksFailedWithoutRunningEngine(t *testing.T) {
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
	require.Equal(t, domain.JobStatusFailed, job.Status)

	scan, err := scans.GetByID(context.Background(), scanID)
	require.NoError(t, err)
	require.Equal(t, domain.ScanStatusCancelled, scan.Status)
}
