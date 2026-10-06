package db

import (
	"context"
	"errors"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

func TestInboxDedupe(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	calls := 0
	fn := func(context.Context, coredb.Tx) error {
		calls++

		return errNil
	}

	process := func() error {
		return withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
			return d.Process(ctx, tx, "evt-1", fn)
		})
	}

	if err := process(); err != nil {
		t.Fatalf("Process(first) error = %v", err)
	}

	if err := process(); err != nil {
		t.Fatalf("Process(repeat) error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("fn calls = %d, want 1 (deduped)", calls)
	}
}

func TestInboxRequiresTx(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := d.Process(t.Context(), nil, "evt", func(context.Context, coredb.Tx) error { return nil })
	if !errors.Is(err, outbox.ErrTxRequired) {
		t.Fatalf("Process(nil tx) = %v, want ErrTxRequired", err)
	}
}

func TestInboxEmptyEventID(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
		return d.Process(ctx, tx, "", func(context.Context, coredb.Tx) error { return nil })
	})
	if !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Process(empty id) = %v, want ErrInvalidMessage", err)
	}
}

func TestInboxRollbackAllowsReprocess(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	boom := errors.New("boom")
	calls := 0

	failing := func(context.Context, coredb.Tx) error {
		calls++

		return boom
	}

	// A failing side effect rolls the whole transaction back, so the event ID
	// is not recorded and a retry runs fn again.
	err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
		return d.Process(ctx, tx, "evt-retry", failing)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Process(failing) = %v, want boom", err)
	}

	ok := func(context.Context, coredb.Tx) error {
		calls++

		return errNil
	}

	if err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
		return d.Process(ctx, tx, "evt-retry", ok)
	}); err != nil {
		t.Fatalf("Process(retry) error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("fn calls = %d, want 2 (rollback released the reservation)", calls)
	}
}
