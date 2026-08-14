//go:build docker

// Run with `go test ./internal/adapters/trivy/... -tags=docker` against a
// real Docker daemon (same convention adapters/sandbox/adapters/dockerx's
// own Docker tests use). ScanConfig's named-volume+subpath mount isn't
// covered here — that mechanism is identical to, and already covered by,
// adapters/sonarqube/scanner.go's own live-verified equivalent; this test
// focuses on what's new to this package: the docker.sock bind that lets
// Trivy see an image on the host daemon, and real report parsing.
package trivy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/dockerx"
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
)

func TestScanner_ScanImage_RealTrivyAgainstAlpine(t *testing.T) {
	docker, err := dockerx.New("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = docker.Close() })
	require.NoError(t, docker.Ping(context.Background()), "Docker daemon must be reachable to run -tags=docker tests")

	require.NoError(t, docker.PullImage(context.Background(), "alpine:3.20"))

	scanner := trivy.NewScanner(docker, trivy.ScannerConfig{
		DBUpdate: true,
		Timeout:  5 * time.Minute,
	})

	report, err := scanner.ScanImage(context.Background(), "alpine:3.20")
	require.NoError(t, err)
	require.NotEmpty(t, report.Results, "trivy image against a real image must report at least one Result (even a clean one names the target)")
}
