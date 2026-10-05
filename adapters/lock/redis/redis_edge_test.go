package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

func TestEdge_UnlockNotHeld(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, nil }},
		prefix: "lock:",
		ttl:    time.Minute,
	}

	h := &handle{a: a, client: a.client, key: "k", holder: "h"}

	if err := h.Unlock(t.Context()); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Unlock = %v, want ErrNotHeld", err)
	}
}

func TestEdge_ExtendNotHeld(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, nil }},
		prefix: "lock:",
		ttl:    time.Minute,
	}

	h := &handle{a: a, client: a.client, key: "k", holder: "h"}

	if err := h.Extend(t.Context(), time.Minute); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("Extend = %v, want ErrNotHeld", err)
	}
}

func TestEdge_TryAcquireDefaultTTL(t *testing.T) {
	t.Parallel()

	var gotTTL time.Duration

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(_ context.Context, _ string, _ any, exp time.Duration) (bool, error) {
				gotTTL = exp
				return true, nil
			},
		},
		prefix: "lock:",
		ttl:    42 * time.Second,
	}

	if _, ok, err := a.TryAcquire(t.Context(), "k", 0); err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	if gotTTL != 42*time.Second {
		t.Fatalf("SetNX ttl = %v, want adapter default %v", gotTTL, 42*time.Second)
	}
}

func TestEdge_ExtendDefaultTTL(t *testing.T) {
	t.Parallel()

	var gotMS int64

	a := &adapter{
		client: &fakeClient{
			evalFn: func(_ context.Context, _ string, _ []string, args ...any) (int64, error) {
				gotMS, _ = args[1].(int64)
				return 1, nil
			},
		},
		prefix: "lock:",
		ttl:    42 * time.Second,
	}

	h := &handle{a: a, client: a.client, key: "k", holder: "h"}

	if err := h.Extend(t.Context(), 0); err != nil {
		t.Fatalf("Extend = %v, want nil", err)
	}

	if gotMS != (42 * time.Second).Milliseconds() {
		t.Fatalf("script ttl = %dms, want %dms", gotMS, (42 * time.Second).Milliseconds())
	}
}

func TestEdge_ConnOptionsURLWhitespace(t *testing.T) {
	t.Parallel()

	got := connOptions(lock.Options{URL: "  redis://h:6379  "})
	if got.Addr != "redis://h:6379" {
		t.Fatalf("Addr = %q, want trimmed URL", got.Addr)
	}
}

func TestEdge_CloseNilRelease(t *testing.T) {
	t.Parallel()

	a := &adapter{}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("first Close = %v, want nil", err)
	}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

func TestEdge_AcquireFreeKeyWithCancelledContext(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) { return true, nil },
			evalFn:  func(context.Context, string, []string, ...any) (int64, error) { return 1, nil },
		},
		prefix:        "lock:",
		ttl:           time.Minute,
		retryInterval: time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Acquire attempts once before observing ctx, so a free key still wins.
	held, err := a.Acquire(ctx, "free", time.Minute)
	if err != nil {
		t.Fatalf("Acquire(free, cancelled) = %v, want nil", err)
	}

	if held == nil {
		t.Fatal("Acquire returned nil lock")
	}
}
