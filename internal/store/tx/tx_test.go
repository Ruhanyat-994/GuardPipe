package tx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/store/tx"
)

// fakeTx is a hand-written fake, not a mock (no mocking framework, per the
// project's testing philosophy). Embedding a nil pgx.Tx satisfies the large
// interface at compile time; only Commit and Rollback are ever actually
// called by WithTx, so only those need real behaviour.
type fakeTx struct {
	pgx.Tx
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (f *fakeTx) Commit(_ context.Context) error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTx) Rollback(_ context.Context) error {
	f.rolledBack = true
	return f.rollbackErr
}

type fakeBeginner struct {
	tx       *fakeTx
	beginErr error
}

func (f *fakeBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

func TestWithTx_CommitsOnSuccess(t *testing.T) {
	txn := &fakeTx{}
	db := &fakeBeginner{tx: txn}

	err := tx.WithTx(context.Background(), db, func(pgx.Tx) error {
		return nil
	})

	if err != nil {
		t.Fatalf("WithTx() error = %v, want nil", err)
	}
	if !txn.committed {
		t.Error("transaction was not committed on success")
	}
	if txn.rolledBack {
		t.Error("transaction was rolled back on success, want commit only")
	}
}

func TestWithTx_RollsBackOnFnError(t *testing.T) {
	txn := &fakeTx{}
	db := &fakeBeginner{tx: txn}
	fnErr := errors.New("insert failed")

	err := tx.WithTx(context.Background(), db, func(pgx.Tx) error {
		return fnErr
	})

	if !errors.Is(err, fnErr) {
		t.Fatalf("WithTx() error = %v, want it to wrap %v", err, fnErr)
	}
	if !txn.rolledBack {
		t.Error("transaction was not rolled back after fn returned an error")
	}
	if txn.committed {
		t.Error("transaction was committed despite fn returning an error")
	}
}

func TestWithTx_BeginErrorNeverCallsFn(t *testing.T) {
	db := &fakeBeginner{beginErr: errors.New("connection refused")}
	called := false

	err := tx.WithTx(context.Background(), db, func(pgx.Tx) error {
		called = true
		return nil
	})

	if err == nil {
		t.Fatal("WithTx() error = nil, want the begin error")
	}
	if called {
		t.Error("fn was called even though Begin failed")
	}
}

func TestWithTx_CommitErrorIsReturned(t *testing.T) {
	txn := &fakeTx{commitErr: errors.New("commit failed: connection reset")}
	db := &fakeBeginner{tx: txn}

	err := tx.WithTx(context.Background(), db, func(pgx.Tx) error {
		return nil
	})

	if err == nil {
		t.Fatal("WithTx() error = nil, want the commit error surfaced")
	}
}

// TestWithTx_PanicRollsBackAndRePanics is the near-miss most transaction
// helpers get wrong: swallowing a panic instead of rolling back and letting
// it continue to propagate, which would leave a caller's recover() (e.g. the
// engine panic-containment wrapper in documentation/04-backend-architecture.md
// §6.4) seeing a transaction that was never actually rolled back.
func TestWithTx_PanicRollsBackAndRePanics(t *testing.T) {
	txn := &fakeTx{}
	db := &fakeBeginner{tx: txn}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected WithTx to re-panic, but it did not panic")
		}
		if r != "boom" {
			t.Errorf("recovered value = %v, want %q", r, "boom")
		}
		if !txn.rolledBack {
			t.Error("transaction was not rolled back before the panic propagated")
		}
		if txn.committed {
			t.Error("transaction was committed despite a panic")
		}
	}()

	_ = tx.WithTx(context.Background(), db, func(pgx.Tx) error {
		panic("boom")
	})
}
