package repo_test

import (
	"context"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/tx"
)

// Compile-time check: *Store must satisfy tx.Beginner, or WithTx(ctx, store, fn)
// stops compiling the moment either side changes shape.
var _ tx.Beginner = (*repo.Store)(nil)

func TestNew_InvalidDSNFailsImmediately(t *testing.T) {
	// No real Postgres needed for this case: pgxpool.ParseConfig rejects a
	// syntactically invalid DSN before attempting any network connection.
	_, err := repo.New(context.Background(), "not a valid postgres url", 25)
	if err == nil {
		t.Fatal("New() error = nil, want an error for a malformed DSN")
	}
}

// TestNew_ValidDSNDoesNotConnectEagerly documents the contract callers rely
// on: pgxpool.New only parses and prepares the pool, it does not dial the
// database. This is what lets Load-then-New succeed even against a database
// that isn't up yet at process start (Compose's dependency ordering handles
// that, not this constructor) — actual reachability is Ping's job, called
// from /readyz.
func TestNew_ValidDSNDoesNotConnectEagerly(t *testing.T) {
	s, err := repo.New(context.Background(), "postgres://user:pass@127.0.0.1:1/guardpipe?sslmode=disable", 5)
	if err != nil {
		t.Fatalf("New() error = %v, want nil for a syntactically valid DSN even against an unreachable host", err)
	}
	defer s.Close()
}
