package orchestrator

import (
	"context"
	"os"
	"sync"

	"github.com/google/uuid"
)

// workspaceCache deduplicates workspace preparation (the repository clone)
// across every job belonging to the same scan. documentation/05-module-specifications.md
// §5's own pipeline diagram draws exactly one "workspace prep" node feeding
// every repository engine (`W --> CS`, `W --> DS`, `W --> K8`, ...) — a
// single clone the whole fan-out reads. Before this existed, processJob
// called prepareWorkspace independently for every job, so an N-engine scan
// cloned the same repository N times, concurrently, from up to N different
// worker goroutines: needless load on the origin (and, on a real host under
// real concurrency, the likely cause of clones failing outright rather than
// succeeding one at a time the way an isolated test would show).
//
// The zero value is ready to use — Pool is normally built as a plain struct
// literal (see worker_test.go), not through a constructor.
type workspaceCache struct {
	mu     sync.Mutex
	byScan map[uuid.UUID]*sharedWorkspace
}

// sharedWorkspace is one scan's in-flight or completed clone. Exactly one
// caller (the first job of a given scan to reach acquire) actually runs
// prepare; every other job for the same scan blocks on ready and then
// reuses dir — including a shared failure, so a bad credential or an
// oversized repo is reported once, not once per engine.
type sharedWorkspace struct {
	ready    chan struct{}
	dir      string
	err      error
	refCount int
}

// acquire returns the shared workspace directory for scanID, running
// prepare at most once no matter how many jobs from the same scan call
// acquire concurrently or in sequence. Every successful acquire must be
// paired with exactly one call to the returned release, made once that job
// is done reading the workspace — the directory is removed only after every
// acquirer has released it, so whichever job happens to finish last (not
// necessarily the one that triggered the clone) is the one that cleans up.
func (c *workspaceCache) acquire(ctx context.Context, scanID uuid.UUID, prepare func() (string, error)) (dir string, release func(), err error) {
	c.mu.Lock()
	ws, exists := c.byScan[scanID]
	if !exists {
		if c.byScan == nil {
			c.byScan = make(map[uuid.UUID]*sharedWorkspace)
		}
		ws = &sharedWorkspace{ready: make(chan struct{})}
		c.byScan[scanID] = ws
	}
	ws.refCount++
	c.mu.Unlock()

	if !exists {
		ws.dir, ws.err = prepare()
		close(ws.ready)
	} else {
		select {
		case <-ws.ready:
		case <-ctx.Done():
			c.releaseRef(scanID, ws)
			return "", nil, ctx.Err()
		}
	}

	if ws.err != nil {
		c.releaseRef(scanID, ws)
		return "", nil, ws.err
	}

	var once sync.Once
	release = func() { once.Do(func() { c.releaseRef(scanID, ws) }) }
	return ws.dir, release, nil
}

func (c *workspaceCache) releaseRef(scanID uuid.UUID, ws *sharedWorkspace) {
	c.mu.Lock()
	ws.refCount--
	done := ws.refCount <= 0
	if done {
		if cur, ok := c.byScan[scanID]; ok && cur == ws {
			delete(c.byScan, scanID)
		}
	}
	c.mu.Unlock()

	if done && ws.dir != "" {
		_ = os.RemoveAll(ws.dir)
	}
}
