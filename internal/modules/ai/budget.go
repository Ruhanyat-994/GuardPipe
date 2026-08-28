package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrBudgetExhausted is returned by BudgetTracker.Reserve when a scan's
// token budget has no room left for the requested amount
// (documentation/10-ai-integration.md §8: "Exhaustion is not an error —
// it returns ErrBudgetExhausted, which callers treat as a skip. A scan
// never fails because of AI budget.").
var ErrBudgetExhausted = errors.New("ai: per-scan token budget exhausted")

// BudgetTracker enforces the per-scan token budget (§8). Reserve is a
// pessimistic reservation made *before* a provider call, using the
// prompt's own MaxTokens as the estimate (the real cost isn't known until
// the response comes back) — Release gives a reservation back afterward
// when it turns out not to have been needed (a cache hit spent nothing
// real).
type BudgetTracker interface {
	Reserve(ctx context.Context, scanID uuid.UUID, amount int) error
	Release(ctx context.Context, scanID uuid.UUID, amount int)
}

// RedisBudgetTracker implements BudgetTracker against `gp:budget:{scan_id}`
// — the exact key documentation/10-ai-integration.md §8 names. Redis is
// explicitly losable by design (CLAUDE.md: "nothing persisted there
// survives restart") — a restart mid-scan just resets everyone's budget
// counter to zero, which is the safe direction to fail (more spend
// allowed afterward, never less).
type RedisBudgetTracker struct {
	client *redis.Client
	limit  int64
	ttl    time.Duration
}

// NewRedisBudgetTracker wires a tracker against perScanLimit tokens (§8's
// documented default is 100,000) — a scan whose enrichment stretches
// across an unusually long time still expires its counter key after ttl
// (24h is generous; no real scan runs anywhere near that long) so an
// abandoned key doesn't live in Redis forever.
func NewRedisBudgetTracker(client *redis.Client, perScanLimit int) *RedisBudgetTracker {
	return &RedisBudgetTracker{client: client, limit: int64(perScanLimit), ttl: 24 * time.Hour}
}

func budgetKey(scanID uuid.UUID) string {
	return "gp:budget:" + scanID.String()
}

// Reserve atomically adds amount to scanID's spent counter (INCRBY, so
// concurrent enrichment calls for the same scan never race each other) and
// fails closed: if the new total exceeds the limit, the reservation is
// rolled back and ErrBudgetExhausted is returned — the caller must not
// make the call it was reserving for.
func (t *RedisBudgetTracker) Reserve(ctx context.Context, scanID uuid.UUID, amount int) error {
	key := budgetKey(scanID)
	spent, err := t.client.IncrBy(ctx, key, int64(amount)).Result()
	if err != nil {
		return fmt.Errorf("ai: reserve budget: %w", err)
	}
	if spent > t.limit {
		t.Release(ctx, scanID, amount)
		return ErrBudgetExhausted
	}
	// Best-effort refresh — an error here just means the key keeps
	// whatever TTL it already had (or none, if this somehow raced its own
	// first INCRBY), never a reason to fail a reservation that already
	// succeeded.
	t.client.Expire(ctx, key, t.ttl)
	return nil
}

// Release gives amount back — used when a reservation turns out not to
// have been needed (Reserve's own rollback on exhaustion, or a caller
// whose call ended up served entirely from cache).
func (t *RedisBudgetTracker) Release(ctx context.Context, scanID uuid.UUID, amount int) {
	// Best-effort: a failed release just means this scan's budget reads a
	// little lower than it really should for the rest of the scan — never
	// worth failing the caller, which has already moved on.
	t.client.DecrBy(ctx, budgetKey(scanID), int64(amount))
}
