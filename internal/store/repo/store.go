// Package repo holds the shared pgx connection pool every per-aggregate
// repository is built on. Per-aggregate repositories themselves (one type
// per aggregate — documentation/06-database-design.md conventions) land
// module by module starting with `identity` in Phase 2; this package only
// owns the connection.
package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the pool every repository shares. It satisfies
// tx.Beginner, so store.WithTx(ctx, store, fn) works directly against it.
type Store struct {
	Pool *pgxpool.Pool
}

// New parses dsn and opens a connection pool with maxConns as the pool
// size ceiling (GUARDPIPE_DB_MAX_CONNS). It does not itself verify
// connectivity — pgxpool.New only parses the DSN and prepares the pool;
// the first real query (or a health check) is what surfaces a genuinely
// unreachable database.
func New(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("repo: parse database URL: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("repo: open connection pool: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close releases every connection in the pool. Safe to call once, at
// shutdown.
func (s *Store) Close() {
	s.Pool.Close()
}

// Ping verifies the database is actually reachable — the check behind
// /readyz (documentation/13-devops-and-environments.md §10, "Health").
func (s *Store) Ping(ctx context.Context) error {
	return s.Pool.Ping(ctx)
}

// Begin starts a transaction on the pool, so Store satisfies
// internal/store/tx.Beginner directly: tx.WithTx(ctx, store, fn).
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.Pool.Begin(ctx)
}
