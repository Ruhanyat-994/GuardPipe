//go:build integration

// Real Redis via testcontainers-go (documentation's testing philosophy:
// "Real PostgreSQL/Redis via testcontainers-go for integration tests —
// mocked SQL tests verify nothing"). Behind the `integration` build tag so
// `go test ./...` stays fast and Docker-independent; run with
// `go test ./internal/modules/advisory/... -tags=integration`.
package advisory_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
)

func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Terminate(context.Background()))
	})

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := redis.ParseURL(connStr)
	require.NoError(t, err)
	return redis.NewClient(opts)
}

func TestRedisCache_MissThenSetThenHit(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()
	cache := advisory.NewRedisCache(client)
	ctx := context.Background()

	_, hit, err := cache.Get(ctx, "gp:cache:osv:npm:left-pad:1.0.0")
	require.NoError(t, err)
	require.False(t, hit)

	want := []advisory.Advisory{{ID: "GHSA-1234", CVE: "CVE-2024-9999", HasFix: true, FixedVersion: "1.0.1"}}
	require.NoError(t, cache.Set(ctx, "gp:cache:osv:npm:left-pad:1.0.0", want, time.Hour))

	got, hit, err := cache.Get(ctx, "gp:cache:osv:npm:left-pad:1.0.0")
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, want, got)
}

func TestRedisCache_EmptyAdvisoryListStaysDistinguishableFromMiss(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()
	cache := advisory.NewRedisCache(client)
	ctx := context.Background()

	require.NoError(t, cache.Set(ctx, "gp:cache:osv:npm:clean-pkg:2.0.0", []advisory.Advisory{}, time.Hour))

	got, hit, err := cache.Get(ctx, "gp:cache:osv:npm:clean-pkg:2.0.0")
	require.NoError(t, err)
	require.True(t, hit)
	require.Empty(t, got)
}

func TestRedisCache_TTLExpires(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()
	cache := advisory.NewRedisCache(client)
	ctx := context.Background()

	require.NoError(t, cache.Set(ctx, "gp:cache:osv:npm:short-lived:1.0.0", []advisory.Advisory{{ID: "x"}}, 50*time.Millisecond))
	time.Sleep(200 * time.Millisecond)

	_, hit, err := cache.Get(ctx, "gp:cache:osv:npm:short-lived:1.0.0")
	require.NoError(t, err)
	require.False(t, hit)
}
