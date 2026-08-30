package queue

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// jobsKey/processingKey match documentation/06-database-design.md §11's
// Redis key table exactly ("every key starts gp:" — the namespace rule).
// documentation/04-backend-architecture.md §6.1's own pseudocode writes
// them as "guardpipe:queue:jobs" — that's illustrative, §11's key table is
// the actual naming contract this follows.
const (
	jobsKey       = "gp:queue:jobs"
	processingKey = "gp:queue:processing"
)

// JobQueue is the reliable-handoff job queue documentation/04-backend-architecture.md
// §6.1 describes: LPUSH to enqueue, BRPOPLPUSH to atomically claim (moving
// a job from the pending list straight into the processing list — no
// window where a claimed job exists nowhere), LREM to ack. Redis holds no
// system of record here — scan_jobs is (§6.1: "If Redis is flushed, a
// recovery sweep re-enqueues all queued/running jobs at startup").
type JobQueue struct {
	client *redis.Client
}

func NewJobQueue(client *redis.Client) *JobQueue {
	return &JobQueue{client: client}
}

// Enqueue pushes a job ID onto the pending list.
func (q *JobQueue) Enqueue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, jobsKey, jobID).Err()
}

// Claim blocks up to timeout for a job, atomically moving it from the
// pending list to the processing list. Returns "", nil (not an error) if
// no job arrives within timeout — the caller's poll loop just tries again.
// Uses BLMOVE RIGHT LEFT rather than the documented BRPOPLPUSH — identical
// semantics (atomic blocking right-pop-then-left-push), but BRPOPLPUSH is
// deprecated in Redis 6.2+ and in this client library.
func (q *JobQueue) Claim(ctx context.Context, timeout time.Duration) (string, error) {
	jobID, err := q.client.BLMove(ctx, jobsKey, processingKey, "RIGHT", "LEFT", timeout).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return jobID, nil
}

// Ack removes one instance of jobID from the processing list — called once
// a job has been durably persisted (findings written, scan_jobs row
// updated), never before.
func (q *JobQueue) Ack(ctx context.Context, jobID string) error {
	return q.client.LRem(ctx, processingKey, 1, jobID).Err()
}

// ListProcessing returns every job ID currently claimed — the reaper's
// input (documentation/04-backend-architecture.md §6.1: "A reaper goroutine
// scans processing every 60s").
func (q *JobQueue) ListProcessing(ctx context.Context) ([]string, error) {
	return q.client.LRange(ctx, processingKey, 0, -1).Result()
}

// JobsInFlight returns how many jobs are currently claimed by a worker —
// satisfies admin.QueueHealthReader for `GET /admin/system-health`
// (BUILD_GUIDE.md Phase 14). LLEN rather than ListProcessing's LRANGE: this
// only needs a count, not the job IDs themselves.
func (q *JobQueue) JobsInFlight(ctx context.Context) (int, error) {
	n, err := q.client.LLen(ctx, processingKey).Result()
	return int(n), err
}

// Requeue moves a job back from processing to pending — the reaper's
// action on a job whose claimed_at (in scan_jobs, the system of record)
// is older than its engine's timeout.
func (q *JobQueue) Requeue(ctx context.Context, jobID string) error {
	if err := q.client.LRem(ctx, processingKey, 1, jobID).Err(); err != nil {
		return err
	}
	return q.client.LPush(ctx, jobsKey, jobID).Err()
}
