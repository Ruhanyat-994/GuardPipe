package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// errScanCancelledByUser is the cause a running engine's context is
// cancelled with when its scan's cancel flag is set (watchForCancel).
var errScanCancelledByUser = errors.New("scan cancelled by user")

// cancelledWhileRunning is the ErrorReason of a job stopped mid-run by a
// cancel request — distinguishes it from a job cancelled before it started
// (refundable) and from one the orphan sweeper closed (workerLost).
const cancelledWhileRunning = "cancelled_while_running"

// workerLost is the ErrorReason of a job the orphan sweeper closed: it was
// still `running` long after its engine's deadline, so the worker running
// it must have died (container restart, crash) without recording a result.
const workerLost = "worker_lost"

// cancelPollInterval is how often a running job checks its scan's cancel
// flag — the longest a user waits between clicking Cancel and the engine
// being told to stop.
var cancelPollInterval = 3 * time.Second

// watchForCancel cancels a running job's context once its scan's cancel
// flag is set. Returns when ctx ends (the engine finished or was stopped).
func (p *Pool) watchForCancel(ctx context.Context, scanID uuid.UUID, cancel context.CancelCauseFunc) {
	ticker := time.NewTicker(cancelPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scan, err := p.Scans.GetByID(ctx, scanID)
			if err != nil {
				continue // transient — the next tick tries again
			}
			if scan.CancelRequested {
				cancel(errScanCancelledByUser)
				return
			}
		}
	}
}

// orphanGrace is how long past its engine's own timeout a job may still
// read `running` before the sweeper decides its worker is gone. A live
// worker always records a result by its deadline (runCtx's timeout), so
// anything older than deadline + grace can't still be running anywhere —
// which is what makes the sweep safe with several worker replicas.
const orphanGrace = 5 * time.Minute

// orphanSweepInterval is how often the sweeper runs.
const orphanSweepInterval = time.Minute

// timeoutFor is the deadline processJob gives an engine.
func (p *Pool) timeoutFor(engine domain.EngineID) time.Duration {
	if t := p.EngineTimeouts[engine]; t > 0 {
		return t
	}
	return p.DefaultTimeout
}

// SweepOrphanedJobs closes jobs whose worker died mid-run: failed with
// worker_lost, or cancelled if the user had asked to cancel the scan. Both
// go through persist, so the scan finalises, is scored, notifies, and the
// job's tokens are refunded exactly like any other ending. Returns how many
// jobs it closed.
func (p *Pool) SweepOrphanedJobs(ctx context.Context) int {
	shortest := p.DefaultTimeout
	for _, t := range p.EngineTimeouts {
		if t > 0 && (shortest <= 0 || t < shortest) {
			shortest = t
		}
	}
	now := time.Now()
	candidates, err := p.Jobs.ListRunningStartedBefore(ctx, now.Add(-(shortest + orphanGrace)))
	if err != nil {
		p.Log.Error("orchestrator: list stale running jobs failed", "error", err)
		return 0
	}

	closed := 0
	for _, job := range candidates {
		if job.StartedAt == nil || now.Sub(*job.StartedAt) < p.timeoutFor(job.Engine)+orphanGrace {
			continue // still within this engine's own (longer) deadline
		}
		scan, err := p.Scans.GetByID(ctx, job.ScanID)
		if err != nil {
			p.Log.Error("orchestrator: load scan for orphaned job failed", "job_id", job.ID, "error", err)
			continue
		}
		status := domain.JobStatusFailed
		if scan.CancelRequested {
			status = domain.JobStatusCancelled
		}
		p.Log.Warn("orchestrator: closing orphaned job", "job_id", job.ID, "scan_id", scan.ID, "engine", job.Engine, "started_at", job.StartedAt, "status", status)
		p.persist(ctx, JobResult{JobID: job.ID, ScanID: scan.ID, ProjectID: scan.ProjectID, Engine: job.Engine, Status: status, ErrorReason: workerLost})
		closed++
	}
	return closed
}

// FinalizeStuckScans closes scans whose jobs all finished but which were
// never marked finished themselves, then scores and notifies each one as
// if its last job had just completed. Returns how many it closed.
func (p *Pool) FinalizeStuckScans(ctx context.Context) int {
	ids, err := p.JobResults.FinalizeStuckScans(ctx)
	if err != nil {
		p.Log.Error("orchestrator: finalize stuck scans failed", "error", err)
	}
	for _, scanID := range ids {
		p.Log.Warn("orchestrator: finalised stuck scan", "scan_id", scanID)
		p.afterFinalize(ctx, scanID)
	}
	return len(ids)
}

// StartOrphanSweeper runs SweepOrphanedJobs now and then every
// orphanSweepInterval until ctx ends. Worker role only.
func (p *Pool) StartOrphanSweeper(ctx context.Context) {
	ticker := time.NewTicker(orphanSweepInterval)
	defer ticker.Stop()
	for {
		if n := p.SweepOrphanedJobs(ctx); n > 0 {
			p.Log.Info("orchestrator: orphaned jobs closed", "count", n)
		}
		p.FinalizeStuckScans(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
