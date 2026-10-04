package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

// TestEdgeTryAcquire_heldKey proves a key held elsewhere reports ok=false
// with a nil error and no handle.
func TestEdgeTryAcquire_heldKey(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) {
				return false, nil
			},
		},
		prefix: "lock:", ttl: time.Second,
	}

	h, ok, err := a.TryAcquire(t.Context(), "k", time.Second)
	if err != nil || ok || h != nil {
		t.Fatalf("TryAcquire(held) = %v, %v, %v; want nil, false, nil", h, ok, err)
	}
}

// TestEdgeExtend_notHeld proves an eval returning 0 maps to ErrNotHeld.
func TestEdgeExtend_notHeld(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, nil }},
		prefix: "lock:", ttl: time.Second,
	}
	h := &handle{a: a, client: a.client, key: "k", holder: "h"}

	if err := h.Extend(t.Context(), 0); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Extend err = %v, want ErrNotHeld", err)
	}
}

// TestEdgeUnlock_notHeld proves an eval returning 0 maps to ErrNotHeld.
func TestEdgeUnlock_notHeld(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, nil }},
		prefix: "lock:", ttl: time.Second,
	}
	h := &handle{a: a, client: a.client, key: "k", holder: "h"}

	if err := h.Unlock(t.Context()); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Unlock err = %v, want ErrNotHeld", err)
	}
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	a := &adapter{}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}
