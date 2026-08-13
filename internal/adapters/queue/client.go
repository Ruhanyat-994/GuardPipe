// Package queue is GuardPipe's sole Redis touchpoint (documentation's
// architecture tree: "adapters/ ... queue(redis)"). Phase 5 only needs the
// connection itself — modules/advisory builds its OSV cache on top of the
// *redis.Client this package constructs. The Redis-backed job queue the
// name promises is Phase 6's `modules/orchestrator` work; this file is the
// piece that has to exist first.
package queue

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// New parses url (GUARDPIPE_REDIS_URL) and returns a connected client. It
// does not verify reachability beyond option parsing — same contract as
// store/repo.New for Postgres — the first real command (or a health check)
// is what surfaces a genuinely unreachable Redis.
func New(url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("queue: parse redis URL: %w", err)
	}
	return redis.NewClient(opts), nil
}

// Ping verifies Redis is actually reachable — the check behind /readyz
// (documentation/13-devops-and-environments.md §10, "Health").
func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
