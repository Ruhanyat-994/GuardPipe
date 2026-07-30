package store

import (
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigrations_ParseCleanly checks that every embedded migration file is
// well-formed goose SQL (has both directions, is sequentially numbered) —
// without needing a live Postgres. This is what would have caught a typo
// like a missing "-- +goose Down" before it ever reached CI's database
// service.
func TestMigrations_ParseCleanly(t *testing.T) {
	goose.SetBaseFS(migrationsFS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("SetDialect() error = %v", err)
	}

	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("CollectMigrations() error = %v", err)
	}

	if len(migrations) < 2 {
		t.Fatalf("got %d migrations, want at least 2 (extensions/enums, organizations/users/refresh_tokens)", len(migrations))
	}

	for i, m := range migrations {
		wantVersion := int64(i + 1)
		if m.Version != wantVersion {
			t.Errorf("migration %d has version %d, want %d — sequence must have no gaps", i, m.Version, wantVersion)
		}
	}
}

// TestMigrations_FirstTwoCoverPhase1Scope pins the two migrations Phase 1
// is scoped to produce, by version number, so a later phase accidentally
// renumbering or duplicating 00001/00002 fails a test instead of silently
// shipping a broken sequence.
func TestMigrations_FirstTwoCoverPhase1Scope(t *testing.T) {
	goose.SetBaseFS(migrationsFS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("SetDialect() error = %v", err)
	}

	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("CollectMigrations() error = %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("got %d migrations, want at least 2", len(migrations))
	}
	if migrations[0].Version != 1 {
		t.Errorf("first migration version = %d, want 1 (extensions + enums)", migrations[0].Version)
	}
	if migrations[1].Version != 2 {
		t.Errorf("second migration version = %d, want 2 (organizations/users/refresh_tokens)", migrations[1].Version)
	}
}
