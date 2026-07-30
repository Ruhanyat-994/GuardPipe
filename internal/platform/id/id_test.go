package id_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

func TestNew_ReturnsValidVersion4UUID(t *testing.T) {
	got := id.New()

	if got == uuid.Nil {
		t.Fatal("New() returned the nil UUID")
	}
	if got.Version() != 4 {
		t.Errorf("New().Version() = %d, want 4", got.Version())
	}
}

func TestNew_IsUniquePerCall(t *testing.T) {
	seen := make(map[uuid.UUID]bool)
	const n = 1000
	for i := range n {
		got := id.New()
		if seen[got] {
			t.Fatalf("New() produced a duplicate after %d calls: %s", i, got)
		}
		seen[got] = true
	}
}
