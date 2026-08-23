package orchestrator

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// LiveProgress is in-process, in-memory shared state between Pool (writes,
// as an engine reports its own real stage progress via
// domain.ScanInput.ReportProgress) and Service.GetProgress (reads, on every
// ~2s poll). In-memory rather than Redis-backed: this deployment runs one
// process per replica (GUARDPIPE_WORKER_COUNT workers inside it, not
// separate worker processes) — see documentation/03-architecture-overview.md
// §5's "Single process, two roles" — so there is exactly one place this
// state could live anyway. A Redis-backed `gp:progress:{scan_id}` hash
// (the shape 07-api-specification.md's own GetProgress doc comment already
// names as the eventual design) is the correct upgrade the day this splits
// into separate API/worker replicas; nothing here forecloses that, since
// callers only see the Get/Set/Clear methods below.
type LiveProgress struct {
	mu  sync.RWMutex
	byJob map[uuid.UUID]jobProgress
}

type jobProgress struct {
	Pct      int
	Activity string
}

func NewLiveProgress() *LiveProgress {
	return &LiveProgress{byJob: make(map[uuid.UUID]jobProgress)}
}

// Set is what an engine's ReportProgress callback (worker.go's closure)
// ultimately calls — pct is clamped to [0,99] so an engine's own
// self-reported progress can never claim the terminal 100% itself; only the
// orchestrator noticing the job actually finished gets to do that (Clear
// below, read as "no live entry" + a terminal job status = 100 in
// GetProgress).
func (p *LiveProgress) Set(jobID uuid.UUID, pct int, activity string) {
	if pct < 0 {
		pct = 0
	}
	if pct > 99 {
		pct = 99
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byJob[jobID] = jobProgress{Pct: pct, Activity: activity}
}

// Get returns the live entry for jobID, if an engine has reported one.
func (p *LiveProgress) Get(jobID uuid.UUID) (jobProgress, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.byJob[jobID]
	return v, ok
}

// Clear removes jobID's entry — called once a job reaches any terminal
// status, so this map never grows past however many jobs are genuinely
// running right now, and a finished job's stale "87%, fuzzing…" can't leak
// into a later re-read.
func (p *LiveProgress) Clear(jobID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.byJob, jobID)
}

// elapsedFallbackPct is the honest answer for the engines that don't (yet)
// report named stages of their own: a real percentage computed from actual
// elapsed wall-clock time against that engine's own configured timeout,
// capped at 95 until the job is genuinely done — never a frozen constant.
// This is presented to the user as real progress, not a fabricated number:
// "45% of the time this engine is allotted has elapsed" is a true fact,
// even though it's a coarser signal than pentest's real phase-by-phase
// reporting. startedAt/timeout must both be valid (checked by the caller).
func elapsedFallbackPct(startedAt time.Time, timeout time.Duration) int {
	if timeout <= 0 {
		return 50
	}
	elapsed := time.Since(startedAt)
	pct := int(elapsed * 100 / timeout)
	return min(max(pct, 1), 95)
}
