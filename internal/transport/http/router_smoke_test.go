package http_test

import (
	"io"
	"log/slog"
	"testing"

	transporthttp "github.com/Ruhanyat-994/GuardPipe/internal/transport/http"
)

// TestRouterConstructsWithoutPanic guards against a Gin route-registration
// conflict (e.g. two routes sharing a path position with differently-named
// wildcard params, or a static route colliding with a wildcard sibling) —
// the kind of bug that otherwise only surfaces as a panic at real server
// startup, not at compile or vet time. Every RouterConfig field is left at
// its zero value deliberately: NewRouter only registers handlers here, it
// never calls into any service, so nil dependencies are safe for this
// purpose and keep the test from needing to construct real ones.
func TestRouterConstructsWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewRouter panicked: %v", r)
		}
	}()
	transporthttp.NewRouter(transporthttp.RouterConfig{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}
