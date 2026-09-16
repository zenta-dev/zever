package db

import (
	"context"
	"errors"
	"testing"
)

type fakeDB struct{}

func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (Rows, error) {
	return &stubRows{}, nil
}

func (f *fakeDB) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 0, nil
}

func (f *fakeDB) Ping(_ context.Context) error { return nil }

func (f *fakeDB) Close(_ context.Context) error { return nil }

func (f *fakeDB) Dialect() string { return "fake" }

type fakeTx struct {
	fakeDB
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

func (f *fakeTx) Savepoint(_ context.Context, _ string) error { return nil }

func (f *fakeTx) RollbackTo(_ context.Context, _ string) error { return nil }

type fakeTransactor struct {
	fakeDB
	tx       *fakeTx
	beginErr error
}

func (f *fakeTransactor) BeginTx(_ context.Context, _ *TxOptions) (Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}

	return f.tx, nil
}

func TestIsolationLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level IsolationLevel
		want  string
	}{
		{name: "read committed", level: ReadCommitted, want: "read committed"},
		{name: "repeatable read", level: RepeatableRead, want: "repeatable read"},
		{name: "serializable", level: Serializable, want: "serializable"},
		{name: "default", level: IsolationLevel(99), want: "IsolationLevel(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.level.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTxOptionsZero(t *testing.T) {
	t.Parallel()

	var opts TxOptions
	if opts.Isolation != ReadCommitted {
		t.Errorf("zero Isolation = %v, want ReadCommitted", opts.Isolation)
	}

	if opts.ReadOnly {
		t.Error("zero ReadOnly should be false")
	}

	if opts.Deferrable {
		t.Error("zero Deferrable should be false")
	}
}

func TestTxContextHelpers(t *testing.T) {
	t.Parallel()

	t.Run("missing returns false", func(t *testing.T) {
		t.Parallel()

		if tx, ok := TxFromContext(context.Background()); ok || tx != nil {
			t.Errorf("got (%v, %v), want (nil, false)", tx, ok)
		}
	})

	t.Run("wrong type returns false", func(t *testing.T) {
		t.Parallel()

		ctx := context.WithValue(context.Background(), txContextKey{}, "not-a-tx")
		if tx, ok := TxFromContext(ctx); ok || tx != nil {
			t.Errorf("got (%v, %v), want (nil, false)", tx, ok)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()

		want := &fakeTx{}
		ctx := WithTxIntoContext(context.Background(), want)

		got, ok := TxFromContext(ctx)
		if !ok {
			t.Fatal("expected tx in context")
		}

		if got != want {
			t.Error("context tx mismatch")
		}
	})
}

func TestWithTx(t *testing.T) {
	t.Parallel()

	t.Run("commit path", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		db := &fakeTransactor{tx: tx}

		err := WithTx(context.Background(), db, nil, func(ctx context.Context, got Tx) error {
			if got != Tx(tx) {
				t.Error("tx mismatch")
			}

			if _, ok := TxFromContext(ctx); !ok {
				t.Error("tx missing from context")
			}

			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !tx.committed {
			t.Error("expected commit")
		}

		if tx.rolledBack {
			t.Error("unexpected rollback")
		}
	})

	t.Run("fn error rolls back", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		db := &fakeTransactor{tx: tx}
		fnErr := errors.New("fn failed")

		err := WithTx(context.Background(), db, nil, func(_ context.Context, _ Tx) error {
			return fnErr
		})
		if !errors.Is(err, fnErr) {
			t.Fatalf("got %v, want fn error", err)
		}

		if !tx.rolledBack {
			t.Error("expected rollback")
		}

		if tx.committed {
			t.Error("unexpected commit")
		}
	})

	t.Run("commit error TxError", func(t *testing.T) {
		t.Parallel()

		commitErr := errors.New("commit down")
		tx := &fakeTx{commitErr: commitErr}
		db := &fakeTransactor{tx: tx}

		err := WithTx(context.Background(), db, nil, func(_ context.Context, _ Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var txErr *TxError
		if !errors.As(err, &txErr) {
			t.Fatalf("error is %T, want *TxError", err)
		}

		if txErr.Op != "commit" {
			t.Errorf("Op = %q, want commit", txErr.Op)
		}

		if !errors.Is(err, commitErr) {
			t.Errorf("does not wrap commit error: %v", err)
		}
	})

	t.Run("begin error TxError", func(t *testing.T) {
		t.Parallel()

		beginErr := errors.New("begin down")
		db := &fakeTransactor{beginErr: beginErr}

		err := WithTx(context.Background(), db, nil, func(_ context.Context, _ Tx) error {
			t.Error("fn should not run")

			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var txErr *TxError
		if !errors.As(err, &txErr) {
			t.Fatalf("error is %T, want *TxError", err)
		}

		if txErr.Op != "begin" {
			t.Errorf("Op = %q, want begin", txErr.Op)
		}

		if !errors.Is(err, beginErr) {
			t.Errorf("does not wrap begin error: %v", err)
		}
	})

	t.Run("nested rejected", func(t *testing.T) {
		t.Parallel()

		db := &fakeTransactor{tx: &fakeTx{}}
		ctx := WithTxIntoContext(context.Background(), &fakeTx{})

		err := WithTx(ctx, db, nil, func(_ context.Context, _ Tx) error {
			t.Error("fn should not run")

			return nil
		})
		if !errors.Is(err, ErrNestedTx) {
			t.Errorf("expected ErrNestedTx, got %v", err)
		}
	})

	t.Run("non transactor", func(t *testing.T) {
		t.Parallel()

		err := WithTx(context.Background(), &fakeDB{}, nil, func(_ context.Context, _ Tx) error {
			t.Error("fn should not run")

			return nil
		})
		if !errors.Is(err, ErrTxUnsupported) {
			t.Errorf("expected ErrTxUnsupported, got %v", err)
		}
	})

	t.Run("panic rolls back and repanics", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		db := &fakeTransactor{tx: tx}

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}

			if r != "boom" {
				t.Errorf("panic = %v, want boom", r)
			}

			if !tx.rolledBack {
				t.Error("expected rollback on panic")
			}

			if tx.committed {
				t.Error("unexpected commit on panic")
			}
		}()

		_ = WithTx(context.Background(), db, nil, func(_ context.Context, _ Tx) error {
			panic("boom")
		})
	})

	t.Run("panic rollback fail still panics", func(t *testing.T) {
		t.Parallel()

		rbErr := errors.New("rollback down")
		tx := &fakeTx{rollbackErr: rbErr}
		db := &fakeTransactor{tx: tx}

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}

			if !tx.rolledBack {
				t.Error("expected rollback attempt")
			}
		}()

		_ = WithTx(context.Background(), db, nil, func(_ context.Context, _ Tx) error {
			panic("boom")
		})
	})
}
