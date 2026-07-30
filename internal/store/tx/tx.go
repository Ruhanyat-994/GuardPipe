// Package tx provides the one transaction helper every repository uses,
// so "one transaction per job/request where the domain requires atomicity"
// is a single reviewed implementation rather than six developers each
// hand-rolling begin/commit/rollback (documentation/03-architecture-overview.md
// §7.7).
package tx

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Beginner is the subset of *pgxpool.Pool (and *pgx.Conn) that WithTx needs.
// Depending on this narrow interface rather than a concrete pool type is
// what makes WithTx testable with a hand-written fake instead of a real
// database (documentation's "no mocking framework" testing philosophy).
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn inside a transaction on db: it begins, calls fn, and
// commits if fn returns nil or rolls back if fn returns an error or panics.
// A panic is rolled back and then re-panicked — WithTx never swallows one.
func WithTx(ctx context.Context, db Beginner, fn func(pgx.Tx) error) (err error) {
	transaction, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tx: begin: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = transaction.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = transaction.Rollback(ctx)
			return
		}
		if commitErr := transaction.Commit(ctx); commitErr != nil {
			err = fmt.Errorf("tx: commit: %w", commitErr)
		}
	}()

	err = fn(transaction)
	return err
}
