package redis

import (
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/cache"
)

// TestEdgeEmptyValue_roundTrip covers the zero-length-value boundary against
// an in-process miniredis server. Sequential by design: New threads through
// the shared client singleton, so t.Parallel is forbidden here.
func TestEdgeEmptyValue_roundTrip(t *testing.T) {
	a, _ := newLiveAdapter(t)

	ctx := t.Context()

	if err := a.Set(ctx, "empty", nil, 0); err != nil {
		t.Fatalf("Set(nil) = %v, want nil", err)
	}

	got, err := a.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Fatalf("Get = %q, want empty", got)
	}
}

// TestEdgeDecrement_nonInteger covers the DECR error mapping: a present but
// non-integer value surfaces cache.ErrInvalidValue / *cache.InvalidValueError.
func TestEdgeDecrement_nonInteger(t *testing.T) {
	a, _ := newLiveAdapter(t)

	ctx := t.Context()

	if err := a.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		t.Fatalf("Set = %v, want nil", err)
	}

	err := a.Decrement(ctx, "bad")
	if !errors.Is(err, cache.ErrInvalidValue) {
		t.Fatalf("Decrement(abc) = %v, want ErrInvalidValue", err)
	}

	var invErr cache.InvalidValueError
	if !errors.As(err, &invErr) {
		t.Fatalf("errors.As(err, InvalidValueError) = false (err = %T %v)", err, err)
	}

	if invErr.Key != "bad" {
		t.Fatalf("InvalidValueError.Key = %q, want bad", invErr.Key)
	}
}

// TestEdgeClose_nilRelease covers Close on an adapter with no registered
// release hook: it must report nil and stay idempotent.
func TestEdgeClose_nilRelease(t *testing.T) {
	t.Parallel()

	a := &redisAdapter{}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("first Close = %v, want nil", err)
	}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

// TestRegisterOpensViaCoreOptions proves Register wires the adapter factory
// into the cache battery registry so cache.Open resolves it. Sequential by
// design: New threads through the shared client singleton.
func TestRegisterOpensViaCoreOptions(t *testing.T) {
	s := miniredis.RunT(t)

	Register()

	c, err := cache.Open(cache.Redis, cache.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	t.Cleanup(func() { _ = c.Close(t.Context()) })

	if setErr := c.Set(t.Context(), "k", []byte("v"), 0); setErr != nil {
		t.Fatalf("Set = %v", setErr)
	}
}
