package orchestrator

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler is BUILD_GUIDE.md Phase 15's "lightweight scheduler ticker" —
// polls scan_schedules for anything due and fires it through
// Service.TriggerSchedule. Runs only under GUARDPIPE_ROLE=worker/all
// (cmd/guardpipe/main.go wires it alongside Pool, reusing the exact same
// role split — no new deployment shape).
type Scheduler struct {
	Schedules    ScanScheduleRepository
	Orchestrator Service
	// Interval is how often the ticker checks for due schedules — not the
	// schedules' own cron cadence. A short poll interval (the default,
	// below) just means a due schedule fires close to its next_run_at, not
	// that scans themselves run more often than their own cron expression.
	Interval time.Duration
	Log      *slog.Logger
}

// DefaultSchedulerInterval is how often the ticker checks for due
// schedules — frequent enough that a schedule fires within a minute of its
// due time, cheap enough (one indexed query) to poll that often.
const DefaultSchedulerInterval = time.Minute

// Start blocks, ticking until ctx is cancelled — the same shape Pool.Start
// already establishes for the worker pool, run the same way
// (cmd/guardpipe/main.go: `go scheduler.Start(ctx)`).
func (s *Scheduler) Start(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = DefaultSchedulerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	due, err := s.Schedules.ListDue(ctx, time.Now().UTC())
	if err != nil {
		if s.Log != nil {
			s.Log.Error("scheduler: list due schedules", "error", err)
		}
		return
	}
	for _, sched := range due {
		if _, err := s.Orchestrator.TriggerSchedule(ctx, sched.ID); err != nil && s.Log != nil {
			// A single bad schedule (deleted project, exhausted engine set)
			// must not stop the rest of the batch from firing — logged and
			// skipped, same "a panicking engine fails only its own job"
			// isolation principle CLAUDE.md's Engine interface doc already
			// states, applied here to one schedule instead of one engine.
			s.Log.Error("scheduler: trigger schedule", "schedule_id", sched.ID, "error", err)
		}
	}
}
