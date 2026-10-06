// Package ratelimittest provides the conformance kit third-party ratelimit adapters run to prove backend parity.
package ratelimittest

import (
	"context"
	"errors"
	"fmt"
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

	if err := checkOpenRegister(); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the open/register round-trip against the
// shared registry. It returns a descriptive error on the first
// contract violation so unit tests can drive every branch.
func checkOpenRegister() error {
	if _, err := ratelimit.Open(ratelimit.Adapter("conformance-missing-adapter"), ratelimit.Options{Rate: 1, Burst: 1}); !errors.Is(err, ratelimit.ErrUnknownAdapter) {
		return fmt.Errorf("Open(missing) err = %w, want ErrUnknownAdapter", err)
	}

	probe := ratelimit.Adapter("conformance-probe-ratelimit")

	if err := ratelimit.Register(probe, nil); !errors.Is(err, ratelimit.ErrNilFactory) {
		return fmt.Errorf("Register(nil) err = %w, want ErrNilFactory", err)
	}

	stub := func(ratelimit.Options) (ratelimit.Limiter, error) {
		return nil, errors.New("ratelimittest: probe factory must not run")
	}

	_ = ratelimit.Register(probe, stub)

	if err := ratelimit.Register(probe, stub); !errors.Is(err, ratelimit.ErrDuplicate) {
		return fmt.Errorf("Register(duplicate) err = %w, want ErrDuplicate", err)
	}

	return nil
}

func conformanceAllowDeny(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	if err := checkAllowDeny(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkAllowDeny proves the bucket drains to a denial carrying a
// positive RetryAfter, and that keys are isolated.
func checkAllowDeny(ctx context.Context, l ratelimit.Limiter) error {
	if l.Name() == "" {
		return errors.New("Name() is empty")
	}

	// Drain the bucket without assuming the factory burst size: keep
	// charging one token until the adapter denies, bounded so a
	// never-denying adapter fails fast instead of looping forever.
	var denied ratelimit.Decision

	deniedAt := -1

	for i := 0; i < 8192; i++ {
		d, err := l.Allow(ctx, "kit-key", 1)
		if err != nil {
			return fmt.Errorf("Allow() error = %w", err)
		}

		if !d.Allowed {
			denied = d
			deniedAt = i

			break
		}
	}

	if deniedAt < 0 {
		return errors.New("Allow() never denied after 8192 charges, want bucket to drain")
	}

	if deniedAt == 0 {
		return errors.New("Allow() denied on a fresh bucket, want at least one charge allowed")
	}

	if denied.RetryAfter <= 0 {
		return fmt.Errorf("RetryAfter = %v, want > 0 on denial", denied.RetryAfter)
	}

	if denied.Remaining < 0 {
		return fmt.Errorf("Remaining = %v, want >= 0", denied.Remaining) //nolint:staticcheck // kit output mirrors the Remaining field name.
	}

	// A different key gets its own bucket.
	other, err := l.Allow(ctx, "kit-other", 1)
	if err != nil {
		return fmt.Errorf("Allow(other) error = %w", err)
	}

	if !other.Allowed {
		return errors.New("Allow(other) Allowed = false, want true (per-key isolation)")
	}

	return nil
}

func conformanceReset(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	if err := checkReset(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkReset proves Reset restores a drained bucket while unknown
// keys reset cleanly.
func checkReset(ctx context.Context, l ratelimit.Limiter) error {
	for i := 0; i < 8192; i++ {
		d, err := l.Allow(ctx, "reset-me", 1)
		if err != nil {
			return fmt.Errorf("Allow() error = %w", err)
		}

		if !d.Allowed {
			break
		}

		if i == 8191 {
			return errors.New("Allow() never denied, want bucket to drain before Reset")
		}
	}

	if d, err := l.Allow(ctx, "reset-me", 1); err != nil || d.Allowed {
		return fmt.Errorf("Allow(over) = %+v,%w want denial,nil", d, err)
	}

	if err := l.Reset(ctx, "reset-me"); err != nil {
		return fmt.Errorf("Reset() error = %w", err)
	}

	if d, err := l.Allow(ctx, "reset-me", 1); err != nil || !d.Allowed {
		return fmt.Errorf("Allow(after reset) = %+v,%w want allowed,nil", d, err)
	}

	if err := l.Reset(ctx, "never-seen"); err != nil {
		return fmt.Errorf("Reset(missing) error = %w, want nil", err)
	}

	return nil
}

func conformanceInvalidInput(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	if err := checkInvalidInput(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidInput proves bad keys and costs surface the invalid
// input sentinels.
func checkInvalidInput(ctx context.Context, l ratelimit.Limiter) error {
	var errs []error

	if _, err := l.Allow(ctx, "", 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
		errs = append(errs, fmt.Errorf("Allow(empty key) err = %w, want ErrInvalidKey", err))
	}

	if _, err := l.Allow(ctx, strings.Repeat("k", ratelimit.MaxKeyLen+1), 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
		errs = append(errs, fmt.Errorf("Allow(long key) err = %w, want ErrInvalidKey", err))
	}

	for _, tokens := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := l.Allow(ctx, "kit-key", tokens); !errors.Is(err, ratelimit.ErrInvalidCost) {
			errs = append(errs, fmt.Errorf("Allow(tokens=%v) err = %w, want ErrInvalidCost", tokens, err))
		}
	}

	if err := l.Reset(ctx, ""); !errors.Is(err, ratelimit.ErrInvalidKey) {
		errs = append(errs, fmt.Errorf("Reset(empty) err = %w, want ErrInvalidKey", err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) ratelimit.Limiter) {
	t.Helper()

	if err := checkClose(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent and the limiter fails closed
// afterwards.
func checkClose(ctx context.Context, l ratelimit.Limiter) error {
	if err := l.Close(); err != nil {
		return fmt.Errorf("Close() error = %w", err)
	}

	if err := l.Close(); err != nil {
		return fmt.Errorf("Close() second error = %w, want nil", err)
	}

	if _, err := l.Allow(ctx, "kit-key", 1); !errors.Is(err, ratelimit.ErrClosed) {
		return fmt.Errorf("Allow() err = %w, want ErrClosed", err)
	}

	if err := l.Reset(ctx, "kit-key"); !errors.Is(err, ratelimit.ErrClosed) {
		return fmt.Errorf("Reset() err = %w, want ErrClosed", err)
	}

	return nil
}
