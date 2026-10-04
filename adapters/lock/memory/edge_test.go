package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/lock"
)

// TestEdgeAcquire_preCancelledContext proves Acquire on a held key returns
// immediately when the context is already done.
func TestEdgeAcquire_preCancelledContext(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})
	ctx := t.Context()

	h, ok, err := l.TryAcquire(ctx, "k", 0)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want ok", ok, err)
	}

	t.Cleanup(func() { _ = h.Unlock(ctx) })

	cctx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := l.Acquire(cctx, "k", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(held, pre-canceled) err = %v, want context.Canceled", err)
	}
}

// TestEdgeExtend_afterUnlock proves a released lease cannot be extended.
func TestEdgeExtend_afterUnlock(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})
	ctx := t.Context()

	h, ok, err := l.TryAcquire(ctx, "k", 0)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = %v, %v, want ok", ok, err)
	}

	if err := h.Unlock(ctx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	if err := h.Extend(ctx, 0); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Extend(after unlock) err = %v, want ErrNotHeld", err)
	}
}

// TestEdgeTryAcquire_emptyKey proves the empty key fails closed.
func TestEdgeTryAcquire_emptyKey(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})

	if _, _, err := l.TryAcquire(t.Context(), "", 0); err == nil {
		t.Fatal("TryAcquire(empty) err = nil, want error")
	}
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	l := newLocker(t, lock.Options{})

	if err := l.Close(t.Context()); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := l.Close(t.Context()); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}
