// Package store owns the database migrations and the small pieces of
// infrastructure every repository builds on: the pgx connection pool and
// the transaction helper (internal/store/tx). Per-aggregate repositories
// (one type per aggregate, per documentation/06-database-design.md
// conventions) are added module by module starting with `identity` in
// Phase 2 — this package holds only what every one of them needs.
package store

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

// Migrate applies every pending goose migration embedded in the binary to
// db. documentation/06-database-design.md §11: migrations are "applied at
// startup" and `/readyz` fails until they complete. Wiring this call into
// the actual startup sequence is Phase 2's job, once main.go holds a real
// pgx-backed *sql.DB — this function is the piece Phase 2 calls.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("store: set goose dialect: %w", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("store: apply migrations: %w", err)
	}
	return nil
}
