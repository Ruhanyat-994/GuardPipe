//go:build docker

// Run with `go test ./internal/adapters/sandbox/... -tags=docker` against a
// real Docker daemon (BUILD_GUIDE.md Phase 5: "Sandbox tests tagged
// -tags=docker so the rest of the suite still runs without Docker"). This
// is the test the phase's "Done when" criterion names: spin up a throwaway
// container, enforce a timeout, confirm it's removed afterward.
package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sandbox"
)

// testImage is a small, widely-cached image good enough for a throwaway
// container. Production callers pin by digest (RunSpec.Image's doc
// comment); a tag is fine for a test that only needs "some container that
// runs a shell command."
const testImage = "alpine:3.20"

func newTestSandbox(t *testing.T) (*sandbox.DockerSandbox, *dockerx.Client) {
	t.Helper()
	docker, err := dockerx.New("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = docker.Close() })

	require.NoError(t, docker.Ping(context.Background()), "Docker daemon must be reachable to run -tags=docker tests")

	return sandbox.New(docker), docker
}

func TestDockerSandbox_Run_Success(t *testing.T) {
	sb, docker := newTestSandbox(t)
	ctx := context.Background()

	result, err := sb.Run(ctx, sandbox.RunSpec{
		Image:   testImage,
		Cmd:     []string{"echo", "hello from sandbox"},
		Network: sandbox.NetworkNone(),
		Timeout: 30 * time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)
	require.Contains(t, string(result.Stdout), "hello from sandbox")
	require.False(t, result.TimedOut)

	ids, err := docker.ListContainerIDsByLabel(ctx, "guardpipe.sandbox", "true")
	require.NoError(t, err)
	require.Empty(t, ids, "the container must be removed after Run returns")
}

func TestDockerSandbox_Run_EnforcesTimeoutAndRemoves(t *testing.T) {
	sb, docker := newTestSandbox(t)
	ctx := context.Background()

	start := time.Now()
	result, err := sb.Run(ctx, sandbox.RunSpec{
		Image:   testImage,
		Cmd:     []string{"sleep", "30"},
		Network: sandbox.NetworkNone(),
		Timeout: 2 * time.Second,
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.True(t, result.TimedOut)
	require.Less(t, elapsed, 15*time.Second, "Run must not wait out the full sleep after its own timeout fires")

	ids, err := docker.ListContainerIDsByLabel(ctx, "guardpipe.sandbox", "true")
	require.NoError(t, err)
	require.Empty(t, ids, "a timed-out container must still be force-removed")
}

func TestDockerSandbox_Run_EnforcesReadOnlyRootfs(t *testing.T) {
	sb, _ := newTestSandbox(t)
	ctx := context.Background()

	result, err := sb.Run(ctx, sandbox.RunSpec{
		Image:   testImage,
		Cmd:     []string{"sh", "-c", "touch /this-should-fail 2>&1; echo exit=$?"},
		Network: sandbox.NetworkNone(),
		Timeout: 30 * time.Second,
	})
	require.NoError(t, err)
	require.NotContains(t, string(result.Stdout), "exit=0", "the root filesystem must be read-only")
}

func TestDockerSandbox_SweepOrphans_RemovesLeakedContainers(t *testing.T) {
	sb, docker := newTestSandbox(t)
	ctx := context.Background()

	require.NoError(t, docker.PullImage(ctx, testImage))
	config := &container.Config{
		Image:  testImage,
		Cmd:    []string{"sleep", "60"},
		Labels: map[string]string{"guardpipe.sandbox": "true"},
	}
	hostConfig := &container.HostConfig{NetworkMode: "none"}
	id, err := docker.CreateContainer(ctx, config, hostConfig, "guardpipe-sandbox-orphan-test")
	require.NoError(t, err)
	require.NoError(t, docker.StartContainer(ctx, id))

	removed, err := sb.SweepOrphans(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, removed, 1)

	ids, err := docker.ListContainerIDsByLabel(ctx, "guardpipe.sandbox", "true")
	require.NoError(t, err)
	require.Empty(t, ids)
}
