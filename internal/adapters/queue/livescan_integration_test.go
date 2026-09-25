//go:build integration

package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/queue"
)

func TestLiveScanStore_DeliveryDedup(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	s := queue.NewLiveScanStore(client)
	ctx := context.Background()

	first, err := s.MarkDeliverySeen(ctx, "delivery-1", time.Minute)
	require.NoError(t, err)
	require.True(t, first)
	again, err := s.MarkDeliverySeen(ctx, "delivery-1", time.Minute)
	require.NoError(t, err)
	require.False(t, again)
}

func TestLiveScanStore_EventQueueIsFIFO(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	s := queue.NewLiveScanStore(client)
	ctx := context.Background()

	require.NoError(t, s.PushEvent(ctx, []byte("a")))
	require.NoError(t, s.PushEvent(ctx, []byte("b")))
	got, err := s.PopEvent(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, "a", string(got))
	got, err = s.PopEvent(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, "b", string(got))

	got, err = s.PopEvent(ctx, 100*time.Millisecond)
	require.NoError(t, err)
	require.Nil(t, got, "empty queue is nil, not an error")
}

func TestLiveScanStore_DebounceKeepsFirstTimerAndLatestPayload(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	s := queue.NewLiveScanStore(client)
	ctx := context.Background()
	start := time.Now()

	require.NoError(t, s.ArmPending(ctx, "p:main", []byte("push-1"), start.Add(30*time.Second)))
	require.NoError(t, s.ArmPending(ctx, "p:main", []byte("push-2"), start.Add(60*time.Second)))
	require.NoError(t, s.ArmPending(ctx, "p:main", []byte("push-3"), start.Add(90*time.Second)))

	due, err := s.PopDuePending(ctx, start.Add(10*time.Second))
	require.NoError(t, err)
	require.Empty(t, due, "window hasn't ended yet")

	// Fires at the FIRST push's deadline, with the LAST push's payload.
	due, err = s.PopDuePending(ctx, start.Add(31*time.Second))
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, "push-3", string(due[0]))

	due, err = s.PopDuePending(ctx, start.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, due, "claimed exactly once")
}

func TestLiveScanStore_WindowCounter(t *testing.T) {
	client := setupTestRedis(t)
	defer func() { _ = client.Close() }()
	s := queue.NewLiveScanStore(client)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		n, err := s.IncrWindowCount(ctx, "project:x", time.Hour)
		require.NoError(t, err)
		require.Equal(t, want, n)
	}
	ttl, err := client.TTL(ctx, "gp:webhook:count:project:x").Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 59*time.Minute)
}
