//go:build integration

// Run with `go test ./internal/modules/ai/... -tags=integration` against a
// real Docker daemon — same convention adapters/queue's own
// jobqueue_integration_test.go already establishes for Redis.
package ai_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)
	opts, err := redis.ParseURL(connStr)
	require.NoError(t, err)
	return redis.NewClient(opts)
}

func TestRedisBudgetTracker_Reserve_WithinLimit_Succeeds(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	tracker := ai.NewRedisBudgetTracker(client, 1000)

	scanID := uuid.New()
	require.NoError(t, tracker.Reserve(context.Background(), scanID, 400))
	require.NoError(t, tracker.Reserve(context.Background(), scanID, 400))
}

// TestRedisBudgetTracker_Reserve_OverLimit_ReturnsErrBudgetExhaustedAndRollsBack
// proves the exhaustion path both fails closed and doesn't leave the
// rejected amount permanently counted against the scan — a caller that
// gets ErrBudgetExhausted and never made its real call must not have
// spent anything.
func TestRedisBudgetTracker_Reserve_OverLimit_ReturnsErrBudgetExhaustedAndRollsBack(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	tracker := ai.NewRedisBudgetTracker(client, 1000)
	scanID := uuid.New()

	require.NoError(t, tracker.Reserve(context.Background(), scanID, 900))

	err := tracker.Reserve(context.Background(), scanID, 200)
	require.ErrorIs(t, err, ai.ErrBudgetExhausted)

	// The rejected 200 must have been rolled back — a subsequent reservation
	// that fits within what's actually left (100) must still succeed.
	require.NoError(t, tracker.Reserve(context.Background(), scanID, 100))
	// And one more unit must now genuinely be over budget.
	require.ErrorIs(t, tracker.Reserve(context.Background(), scanID, 1), ai.ErrBudgetExhausted)
}

func TestRedisBudgetTracker_Release_GivesBudgetBack(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	tracker := ai.NewRedisBudgetTracker(client, 1000)
	scanID := uuid.New()

	require.NoError(t, tracker.Reserve(context.Background(), scanID, 1000))
	require.ErrorIs(t, tracker.Reserve(context.Background(), scanID, 1), ai.ErrBudgetExhausted)

	tracker.Release(context.Background(), scanID, 500)
	require.NoError(t, tracker.Reserve(context.Background(), scanID, 500))
}

// TestRedisBudgetTracker_DifferentScans_HaveIndependentBudgets proves the
// per-scan key isolation (`gp:budget:{scan_id}`) — one scan running out
// must never affect another's.
func TestRedisBudgetTracker_DifferentScans_HaveIndependentBudgets(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	tracker := ai.NewRedisBudgetTracker(client, 100)

	scanA, scanB := uuid.New(), uuid.New()
	require.NoError(t, tracker.Reserve(context.Background(), scanA, 100))
	require.ErrorIs(t, tracker.Reserve(context.Background(), scanA, 1), ai.ErrBudgetExhausted)

	// scanB's own budget is untouched by scanA's exhaustion.
	require.NoError(t, tracker.Reserve(context.Background(), scanB, 100))
}
