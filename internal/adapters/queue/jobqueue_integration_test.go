//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/queue"
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

func TestJobQueue_EnqueueClaimAck_RoundTrips(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	q := queue.NewJobQueue(client)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "job-1"))

	claimed, err := q.Claim(ctx, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, "job-1", claimed)

	processing, err := q.ListProcessing(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"job-1"}, processing)

	require.NoError(t, q.Ack(ctx, "job-1"))
	processing, err = q.ListProcessing(ctx)
	require.NoError(t, err)
	require.Empty(t, processing)
}

func TestJobQueue_Claim_TimesOutWithNoJob(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	q := queue.NewJobQueue(client)

	claimed, err := q.Claim(context.Background(), 500*time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, claimed)
}

func TestJobQueue_Requeue_MovesJobBackToPending(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	q := queue.NewJobQueue(client)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "job-2"))
	_, err := q.Claim(ctx, 2*time.Second)
	require.NoError(t, err)

	require.NoError(t, q.Requeue(ctx, "job-2"))

	processing, err := q.ListProcessing(ctx)
	require.NoError(t, err)
	require.Empty(t, processing, "requeued job must leave the processing list")

	reclaimed, err := q.Claim(ctx, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, "job-2", reclaimed, "requeued job must be claimable again")
}
