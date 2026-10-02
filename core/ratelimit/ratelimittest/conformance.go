// Package ratelimittest provides the conformance kit third-party ratelimit adapters run to prove backend parity.
package ratelimittest

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ratelimit"
)

// Conformance verifies factory-built limiters implement the
// ratelimit.Limiter contract: open/register round-trip, burst allow
// then denial with RetryAfter, Reset restoration, invalid key/cost
// sentinels, Name, and Close. Each subtest takes a fresh instance from
// factory so cases stay isolated. Proof factories should configure a
// small burst (e.g. Burst 3) so the allow-then-deny sequence stays
// deterministic without waiting for refills. Tests never call
// time.Sleep and never touch the network.
//
// No-op adapters: none; every adapter must enforce the token bucket.
// A stub that always allows would trivially satisfy the shape but
// violate the limit, so no stub exemption exists.
func Conformance(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("AllowDeny", func(t *testing.T) { conformanceAllowDeny(t, factory) })
	t.Run("Reset", func(t *testing.T) { conformanceReset(t, factory) })
	t.Run("InvalidInput", func(t *testing.T) { conformanceInvalidInput(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := ratelimit.Open(ratelimit.Adapter("conformance-missing-adapter"), ratelimit.Options{Rate: 1, Burst: 1}); !errors.Is(err, ratelimit.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := ratelimit.Adapter("conformance-probe-ratelimit")

	if err := ratelimit.Register(probe, nil); !errors.Is(err, ratelimit.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(ratelimit.Options) (ratelimit.Limiter, error) {
		return nil, errors.New("ratelimittest: probe factory must not run")
	}

	_ = ratelimit.Register(probe, stub)

	if err := ratelimit.Register(probe, stub); !errors.Is(err, ratelimit.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceAllowDeny(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	if l.Name() == "" {
		t.Error("Name() is empty")
	}

	// Drain the bucket without assuming the factory burst size: keep
	// charging one token until the adapter denies, bounded so a
	// never-denying adapter fails fast instead of looping forever.
	var denied ratelimit.Decision

	deniedAt := -1

	for i := 0; i < 8192; i++ {
		d, err := l.Allow(ctx, "kit-key", 1)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}

		if !d.Allowed {
			denied = d
			deniedAt = i
			break
		}
	}

	if deniedAt < 0 {
		t.Fatal("Allow() never denied after 8192 charges, want bucket to drain")
	}

	if deniedAt == 0 {
		t.Fatal("Allow() denied on a fresh bucket, want at least one charge allowed")
	}

	if denied.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0 on denial", denied.RetryAfter)
	}

	if denied.Remaining < 0 {
		t.Errorf("Remaining = %v, want >= 0", denied.Remaining)
	}

	// A different key gets its own bucket.
	other, err := l.Allow(ctx, "kit-other", 1)
	if err != nil {
		t.Fatalf("Allow(other) error = %v", err)
	}

	if !other.Allowed {
		t.Error("Allow(other) Allowed = false, want true (per-key isolation)")
	}
}

func conformanceReset(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	for i := 0; i < 8192; i++ {
		d, err := l.Allow(ctx, "reset-me", 1)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}

		if !d.Allowed {
			break
		}

		if i == 8191 {
			t.Fatal("Allow() never denied, want bucket to drain before Reset")
		}
	}

	if d, err := l.Allow(ctx, "reset-me", 1); err != nil || d.Allowed {
		t.Fatalf("Allow(over) = %+v,%v want denial,nil", d, err)
	}

	if err := l.Reset(ctx, "reset-me"); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if d, err := l.Allow(ctx, "reset-me", 1); err != nil || !d.Allowed {
		t.Errorf("Allow(after reset) = %+v,%v want allowed,nil", d, err)
	}

	if err := l.Reset(ctx, "never-seen"); err != nil {
		t.Errorf("Reset(missing) error = %v, want nil", err)
	}
}

func conformanceInvalidInput(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	if _, err := l.Allow(ctx, "", 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
		t.Errorf("Allow(empty key) err = %v, want ErrInvalidKey", err)
	}

	if _, err := l.Allow(ctx, strings.Repeat("k", ratelimit.MaxKeyLen+1), 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
		t.Errorf("Allow(long key) err = %v, want ErrInvalidKey", err)
	}

	for _, tokens := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := l.Allow(ctx, "kit-key", tokens); !errors.Is(err, ratelimit.ErrInvalidCost) {
			t.Errorf("Allow(tokens=%v) err = %v, want ErrInvalidCost", tokens, err)
		}
	}

	if err := l.Reset(ctx, ""); !errors.Is(err, ratelimit.ErrInvalidKey) {
		t.Errorf("Reset(empty) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := l.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := l.Allow(ctx, "kit-key", 1); !errors.Is(err, ratelimit.ErrClosed) {
		t.Errorf("Allow() err = %v, want ErrClosed", err)
	}

	if err := l.Reset(ctx, "kit-key"); !errors.Is(err, ratelimit.ErrClosed) {
		t.Errorf("Reset() err = %v, want ErrClosed", err)
	}
}
