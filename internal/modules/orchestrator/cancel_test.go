package orchestrator_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// blockingEngine emits one finding, then runs until its context ends —
// a long engine (pentest, a big SonarQube analysis) mid-run.
type blockingEngine struct {
	id      domain.EngineID
	started atomic.Bool
}

func (e *blockingEngine) ID() domain.EngineID { return e.id }
func (e *blockingEngine) Applicable(context.Context, domain.ScanInput) (bool, string) {
	return true, ""
}
func (e *blockingEngine) Run(ctx context.Context, _ domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	emit(domain.Finding{ID: id.New(), RuleID: "depscan.hygiene.no-lockfile", Severity: domain.SeverityMedium})
	e.started.Store(true)
	<-ctx.Done()
	return domain.EngineResult{}, ctx.Err()
}

// TestPool_CancelRequest_StopsRunningEngine: before this fix a cancel only
// affected jobs not yet started — a running engine kept going, and the scan
// kept showing "running" until it finished on its own.
func TestPool_CancelRequest_StopsRunningEngine(t *testing.T) {
	defer orchestrator.SetCancelPollInterval(20 * time.Millisecond)()
	engine := &blockingEngine{id: domain.EngineDepScan}
	pool, scans, jobs, findings, q := newTestPool(t, engine, &fakeCloner{})
	tokens := newFakeTokens()
	pool.Tokens = tokens
	scanID, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		for !engine.started.Load() {
			time.Sleep(5 * time.Millisecond)
		}
		_ = scans.SetCancelRequested(context.Background(), scanID)
	}()
	done := make(chan struct{})
	go func() { pool.Start(ctx); close(done) }()

	require.Eventually(t, func() bool {
		j, err := jobs.GetByID(context.Background(), jobID)
		return err == nil && j.Status == domain.JobStatusCancelled
	}, time.Second, 10*time.Millisecond, "the running engine must be stopped well before its 5s timeout")
	cancel()
	<-done

	j, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, "cancelled_while_running", *j.ErrorReason)
	n, err := findings.CountByJob(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, 1, n, "what it found before being stopped is kept")
	_, refunded := tokens.refunds[jobID]
	require.False(t, refunded, "a job stopped mid-run did use the engine — not refunded")
}

// Near-miss: without a cancel request, the watcher must never stop an engine.
func TestPool_NoCancelRequest_EngineRunsToCompletion(t *testing.T) {
	defer orchestrator.SetCancelPollInterval(10 * time.Millisecond)()
	engine := &scriptedEngine{id: domain.EngineDepScan, applicable: true}
	pool, scans, jobs, _, q := newTestPool(t, engine, &fakeCloner{})
	_, jobID := seedScanAndJob(t, scans, jobs, domain.EngineDepScan)
	q.pending = []string{jobID.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	j, err := jobs.GetByID(context.Background(), jobID)
	require.NoError(t, err)
	require.Equal(t, domain.JobStatusSucceeded, j.Status)
}

func runningJob(t *testing.T, scans *fakeScanRepo, jobs *fakeScanJobRepo, engine domain.EngineID, startedAgo time.Duration) (scanID, jobID uuid.UUID) {
	t.Helper()
	scanID, jobID = seedScanAndJob(t, scans, jobs, engine)
	require.NoError(t, jobs.MarkRunning(context.Background(), jobID))
	started := time.Now().Add(-startedAgo)
	jobs.mu.Lock()
	jobs.byID[jobID].StartedAt = &started
	jobs.mu.Unlock()
	return scanID, jobID
}

func TestSweepOrphanedJobs(t *testing.T) {
	pool, scans, jobs, _, _ := newTestPool(t, &scriptedEngine{id: domain.EngineDepScan}, &fakeCloner{})
	pool.DefaultTimeout = 10 * time.Minute
	pool.EngineTimeouts = map[domain.EngineID]time.Duration{domain.EnginePentest: 2 * time.Hour}
	tokens := newFakeTokens()
	pool.Tokens = tokens

	// A depscan job its worker abandoned a month ago → failed, refunded,
	// its scan finalised and notified.
	_, lost := runningJob(t, scans, jobs, domain.EngineDepScan, 30*24*time.Hour)
	// Same, but the user already asked to cancel → cancelled.
	cancelScan, cancelledJob := runningJob(t, scans, jobs, domain.EngineDepScan, time.Hour)
	require.NoError(t, scans.SetCancelRequested(context.Background(), cancelScan))
	// Near-misses: still inside their own deadline + grace, must be left alone.
	_, recent := runningJob(t, scans, jobs, domain.EngineDepScan, 12*time.Minute)
	_, longPentest := runningJob(t, scans, jobs, domain.EnginePentest, time.Hour)

	require.Equal(t, 2, pool.SweepOrphanedJobs(context.Background()))

	status := func(jobID uuid.UUID) domain.JobStatus {
		j, err := jobs.GetByID(context.Background(), jobID)
		require.NoError(t, err)
		return j.Status
	}
	require.Equal(t, domain.JobStatusFailed, status(lost))
	require.Equal(t, domain.JobStatusCancelled, status(cancelledJob))
	require.Equal(t, domain.JobStatusRunning, status(recent))
	require.Equal(t, domain.JobStatusRunning, status(longPentest))
	require.Contains(t, tokens.refunds, lost)
	require.Contains(t, tokens.refunds, cancelledJob, "an orphan never did the work — refunded even though cancelled")

	require.Zero(t, pool.SweepOrphanedJobs(context.Background()), "idempotent")
}
