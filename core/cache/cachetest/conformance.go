// Package cachetest provides the conformance kit third-party cache adapters run to prove backend parity.
package cachetest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

const (
	// DefaultEntryTTL is the short TTL conformance expiry tests set before polling for disappearance.
	// It must comfortably exceed the Set→Get round-trip so the "before expiry"
	// assertion cannot race expiry under CI load (a 30ms TTL flaked there).
	DefaultEntryTTL = 1 * time.Second
	// DefaultExpiryTimeout bounds how long expiry polls wait before failing.
	DefaultExpiryTimeout = 5 * time.Second
	// DefaultPollInterval is the tick between expiry-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

// errf builds a non-wrapping descriptive error with Sprintf semantics.
// Check helpers use it (instead of fmt.Errorf with %w) for diagnostics
// where the formatted error may be nil: %w of a nil error prints
// "%!w(<nil>)", diverging from the historical Fatalf text, and errorlint
// forbids %v of an error in Errorf. Failure text stays byte-identical.
// It deliberately avoids fmt.Errorf so only real failures wrap.
func errf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)

	return errors.New(msg)
}

// Conformance verifies factory-built caches implement the cache.Cache contract:
// Get/Set round-trip, TTL expiry, SetIfAbsent, Delete, Increment/Decrement,
// Exists, and Close. Each subtest takes a fresh instance from factory so
// cases stay isolated. Expiry waits poll with a context deadline; they never
// synchronize with time.Sleep and never touch the network.
//
// Virtual-time fakes: when the factory product also implements
// FastForward(time.Duration) (e.g. a miniredis wrapper whose TTLs advance
// only via FastForward), expiry polls advance that clock by the poll
// interval after each unsuccessful attempt so TTLs expire without wall-clock
// waiting. Real-time adapters do not implement it and are unaffected.
func Conformance(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	t.Run("GetSet", func(t *testing.T) { conformanceGetSet(t, factory) })
	t.Run("TTLExpiry", func(t *testing.T) { conformanceTTLExpiry(t, factory) })
	t.Run("SetIfAbsent", func(t *testing.T) { conformanceSetIfAbsent(t, factory) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("IncrementDecrement", func(t *testing.T) { conformanceCounters(t, factory) })
	t.Run("Exists", func(t *testing.T) { conformanceExists(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceGetSet(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkGetSet(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkGetSet proves Get/Set round-trip with defensive copies both ways
// plus overwrite. The first violation aborts; soft value mismatches join.
func checkGetSet(ctx context.Context, c cache.Cache) error {
	if _, err := c.Get(ctx, "missing"); !errors.Is(err, cache.ErrNotFound) {
		return errf("Get(missing) err = %v, want ErrNotFound", err)
	}

	var nfErr cache.NotFoundError
	if _, err := c.Get(ctx, "missing"); !errors.As(err, &nfErr) {
		return errf("errors.As(err, NotFoundError) = false (err = %T %v)", err, err)
	}

	var errs []error

	if nfErr.Key != "missing" {
		errs = append(errs, fmt.Errorf("NotFoundError.Key = %q, want missing", nfErr.Key))
	}

	val := []byte("v1")
	if err := c.Set(ctx, "k", val, 0); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	val[0] = 'X'

	got, err := c.Get(ctx, "k")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(got) != "v1" {
		return fmt.Errorf("Get() = %q, want v1 (stored copy)", got)
	}

	got[0] = 'Y'

	again, err := c.Get(ctx, "k")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(again) != "v1" {
		return fmt.Errorf("Get() = %q, want v1 (returned copy)", again)
	}

	if err = c.Set(ctx, "k", []byte("v2"), 0); err != nil {
		return fmt.Errorf("Set() overwrite error = %w", err)
	}

	got, err = c.Get(ctx, "k")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(got) != "v2" {
		errs = append(errs, fmt.Errorf("Get() = %q, want v2", got))
	}

	return errors.Join(errs...)
}

func conformanceTTLExpiry(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkTTLExpiry(t.Context(), factory(t), DefaultExpiryTimeout); err != nil {
		t.Fatal(err)
	}
}

// checkTTLExpiry proves a TTL entry is readable before expiry, vanishes
// from Get and Exists after polling, and that TTL-zero entries persist.
// Timeout/interval are parameters so unit tests drive both branches fast.
func checkTTLExpiry(ctx context.Context, c cache.Cache, timeout time.Duration) error {
	if err := c.Set(ctx, "k", []byte("v"), DefaultEntryTTL); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	if _, err := c.Get(ctx, "k"); err != nil {
		return fmt.Errorf("Get() before expiry error = %w", err)
	}

	if err := pollExpiry(ctx, timeout, "key expired from Get", func(ctx context.Context) bool {
		_, err := c.Get(ctx, "k")
		if errors.Is(err, cache.ErrNotFound) {
			return true
		}

		maybeFastForward(c)

		return false
	}); err != nil {
		return err
	}

	if err := pollExpiry(ctx, timeout, "exists false after expiry", func(ctx context.Context) bool {
		ok, _ := c.Exists(ctx, "k")
		if !ok {
			return true
		}

		maybeFastForward(c)

		return false
	}); err != nil {
		return err
	}

	if err := c.Set(ctx, "keep", []byte("v"), 0); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	if _, err := c.Get(ctx, "keep"); err != nil {
		return fmt.Errorf("Get(keep) error = %w, want retained (no ttl)", err)
	}

	return nil
}

func conformanceSetIfAbsent(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkSetIfAbsent(t.Context(), factory(t), DefaultExpiryTimeout); err != nil {
		t.Fatal(err)
	}
}

// checkSetIfAbsent proves first-write-wins, overwrite refusal, and that
// an expired key is claimable again after polling.
func checkSetIfAbsent(ctx context.Context, c cache.Cache, timeout time.Duration) error {
	ok, err := c.SetIfAbsent(ctx, "k", []byte("v1"), 0)
	if err != nil || !ok {
		return errf("SetIfAbsent() = %v,%v want true,nil", ok, err)
	}

	ok, err = c.SetIfAbsent(ctx, "k", []byte("v2"), 0)
	if err != nil || ok {
		return errf("SetIfAbsent() second = %v,%v want false,nil", ok, err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(got) != "v1" {
		return fmt.Errorf("Get() = %q, want v1", got)
	}

	if err = c.Set(ctx, "e", []byte("old"), DefaultEntryTTL); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	if pollErr := pollExpiry(ctx, timeout, "expired key accepted by SetIfAbsent", func(ctx context.Context) bool {
		ok, _ := c.SetIfAbsent(ctx, "e", []byte("new"), 0)
		if ok {
			return true
		}

		maybeFastForward(c)

		return false
	}); pollErr != nil {
		return pollErr
	}

	got, err = c.Get(ctx, "e")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(got) != "new" {
		return fmt.Errorf("Get() = %q, want new", got)
	}

	return nil
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkDelete(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkDelete proves Delete is idempotent and that deleted keys read
// back missing. Soft mismatches join so all are reported.
func checkDelete(ctx context.Context, c cache.Cache) error {
	if err := c.Delete(ctx, "missing"); err != nil {
		return fmt.Errorf("Delete(missing) error = %w, want nil", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		return fmt.Errorf("Delete() error = %w", err)
	}

	var errs []error

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		errs = append(errs, errf("errors.Is(err, ErrNotFound) = false (err = %v)", err))
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || ok {
		errs = append(errs, errf("Exists() = %v,%v want false,nil", ok, err))
	}

	if err := c.Delete(ctx, "k"); err != nil {
		errs = append(errs, fmt.Errorf("Delete(again) error = %w, want nil", err))
	}

	return errors.Join(errs...)
}

func conformanceCounters(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkCounters(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkCounters proves Increment from missing starts at 1, Increment and
// Decrement compose, Decrement from missing starts at -1, numeric strings
// increment, and non-numeric values report ErrInvalidValue with the key.
func checkCounters(ctx context.Context, c cache.Cache) error {
	if err := c.Increment(ctx, "n"); err != nil {
		return fmt.Errorf("Increment() error = %w", err)
	}

	got, err := c.Get(ctx, "n")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	if string(got) != "1" {
		return fmt.Errorf("Get() = %q, want 1", got)
	}

	if err = c.Increment(ctx, "n"); err != nil {
		return fmt.Errorf("Increment() error = %w", err)
	}

	if err = c.Decrement(ctx, "n"); err != nil {
		return fmt.Errorf("Decrement() error = %w", err)
	}

	got, err = c.Get(ctx, "n")
	if err != nil {
		return fmt.Errorf("Get() error = %w", err)
	}

	var errs []error

	if string(got) != "1" {
		errs = append(errs, fmt.Errorf("Get() = %q, want 1", got))
	}

	if err = c.Decrement(ctx, "fresh"); err != nil {
		errs = append(errs, fmt.Errorf("Decrement() error = %w", err))
		return errors.Join(errs...)
	}

	got, err = c.Get(ctx, "fresh")
	if err != nil {
		errs = append(errs, fmt.Errorf("Get() error = %w", err))
		return errors.Join(errs...)
	}

	if string(got) != "-1" {
		errs = append(errs, fmt.Errorf("Get() = %q, want -1 (missing base)", got))
	}

	if err = c.Set(ctx, "base", []byte("41"), 0); err != nil {
		errs = append(errs, fmt.Errorf("Set() error = %w", err))
		return errors.Join(errs...)
	}

	if err = c.Increment(ctx, "base"); err != nil {
		errs = append(errs, fmt.Errorf("Increment() error = %w", err))
		return errors.Join(errs...)
	}

	got, err = c.Get(ctx, "base")
	if err != nil {
		errs = append(errs, fmt.Errorf("Get() error = %w", err))
		return errors.Join(errs...)
	}

	if string(got) != "42" {
		errs = append(errs, fmt.Errorf("Get() = %q, want 42", got))
	}

	if err = c.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		errs = append(errs, fmt.Errorf("Set() error = %w", err))
		return errors.Join(errs...)
	}

	err = c.Increment(ctx, "bad")
	if err == nil {
		errs = append(errs, errors.New("Increment(abc) = nil, want ErrInvalidValue"))
		return errors.Join(errs...)
	}

	if !errors.Is(err, cache.ErrInvalidValue) {
		errs = append(errs, errf("errors.Is(err, ErrInvalidValue) = false (err = %v)", err))
	}

	var invErr cache.InvalidValueError
	if !errors.As(err, &invErr) {
		errs = append(errs, errf("errors.As(err, InvalidValueError) = false (err = %T %v)", err, err))
		return errors.Join(errs...)
	}

	if invErr.Key != "bad" {
		errs = append(errs, fmt.Errorf("InvalidValueError.Key = %q, want bad", invErr.Key))
	}

	return errors.Join(errs...)
}

func conformanceExists(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkExists(t.Context(), factory(t), DefaultExpiryTimeout); err != nil {
		t.Fatal(err)
	}
}

// checkExists proves Exists tracks Set/Delete and flips false after TTL
// expiry polling.
func checkExists(ctx context.Context, c cache.Cache, timeout time.Duration) error {
	if ok, err := c.Exists(ctx, "missing"); err != nil || ok {
		return errf("Exists() = %v,%v want false,nil", ok, err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || !ok {
		return errf("Exists() = %v,%v want true,nil", ok, err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		return fmt.Errorf("Delete() error = %w", err)
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || ok {
		return errf("Exists() after delete = %v,%v want false,nil", ok, err)
	}

	if err := c.Set(ctx, "e", []byte("v"), DefaultEntryTTL); err != nil {
		return fmt.Errorf("Set() error = %w", err)
	}

	return pollExpiry(ctx, timeout, "exists false for expired entry", func(ctx context.Context) bool {
		ok, err := c.Exists(ctx, "e")
		if err == nil && !ok {
			return true
		}

		maybeFastForward(c)

		return false
	})
}

func conformanceClose(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	if err := checkClose(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent and every method reports
// ErrClosed afterwards. Soft mismatches join so all are reported.
func checkClose(ctx context.Context, c cache.Cache) error {
	if err := c.Close(ctx); err != nil {
		return fmt.Errorf("Close() error = %w", err)
	}

	var errs []error

	if err := c.Close(ctx); err != nil {
		errs = append(errs, fmt.Errorf("Close() second error = %w, want nil", err))
	}

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Get() err = %v, want ErrClosed", err))
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Set() err = %v, want ErrClosed", err))
	}

	if _, err := c.SetIfAbsent(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("SetIfAbsent() err = %v, want ErrClosed", err))
	}

	if err := c.Delete(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Delete() err = %v, want ErrClosed", err))
	}

	if err := c.Increment(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Increment() err = %v, want ErrClosed", err))
	}

	if err := c.Decrement(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Decrement() err = %v, want ErrClosed", err))
	}

	if _, err := c.Exists(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		errs = append(errs, errf("Exists() err = %v, want ErrClosed", err))
	}

	return errors.Join(errs...)
}

// fastForwarder is implemented by conformance factories running against
// virtual-time fakes (e.g. a miniredis wrapper whose TTLs advance only via
// FastForward). Expiry polls advance it after each unsuccessful attempt.
type fastForwarder interface {
	FastForward(time.Duration)
}

// maybeFastForward advances c's virtual clock when it implements
// fastForwarder; it is a no-op for real-time adapters.
func maybeFastForward(c cache.Cache) {
	if f, ok := any(c).(fastForwarder); ok {
		f.FastForward(DefaultPollInterval)
	}
}

// pollExpiry polls cond until true or timeout elapses, ticking every
// interval. It returns a descriptive error on timeout so check helpers
// can propagate it without touching *testing.T. (It replaces the old
// eventually(t, ...) helper, whose timeout branch was uncoverable: a
// *testing.T failure cannot be scripted without a real test run.)
func pollExpiry(ctx context.Context, timeout time.Duration, msg string, cond func(ctx context.Context) bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("condition not met within %v: %s", timeout, msg)
		case <-ticker.C:
		}
	}
}
