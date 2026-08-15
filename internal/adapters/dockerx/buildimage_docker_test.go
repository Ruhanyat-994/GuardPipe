//go:build docker

// Run with `go test ./internal/adapters/dockerx/... -tags=docker` against a
// real Docker daemon (same convention adapters/sandbox's own Docker tests
// use).
package dockerx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

func newTestClient(t *testing.T) *dockerx.Client {
	t.Helper()
	c, err := dockerx.New("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.Ping(context.Background()), "Docker daemon must be reachable to run -tags=docker tests")
	return c
}

func TestClient_BuildImage_Success(t *testing.T) {
	docker := newTestClient(t)
	ctx := context.Background()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:3.20\nRUN echo hello > /hello.txt\n"), 0o644))

	tag := "guardpipe-dockerx-test-" + id.New().String() + ":latest"
	err := docker.BuildImage(ctx, dir, "Dockerfile", tag)
	require.NoError(t, err)
	t.Cleanup(func() { _ = docker.RemoveImage(context.Background(), tag) })

	// A real inspect-equivalent isn't wrapped by dockerx yet — proving the
	// tag now exists and is removable is enough to confirm the build
	// actually produced a usable image, not just a no-op success.
	require.NoError(t, docker.RemoveImage(ctx, tag), "the image must exist and be removable after a successful build")
}

func TestClient_BuildImage_FailsOnBadDockerfile(t *testing.T) {
	docker := newTestClient(t)
	ctx := context.Background()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("NOT_A_REAL_INSTRUCTION oops\n"), 0o644))

	tag := "guardpipe-dockerx-test-" + id.New().String() + ":latest"
	err := docker.BuildImage(ctx, dir, "Dockerfile", tag)
	require.Error(t, err, "an invalid Dockerfile must surface as a BuildImage error, not a silent empty image")
}
