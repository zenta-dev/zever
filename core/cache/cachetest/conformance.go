// Package cachetest provides the conformance kit third-party cache adapters run to prove backend parity.
package cachetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

const (
	// DefaultEntryTTL is the short TTL conformance expiry tests set before polling for disappearance.
	DefaultEntryTTL = 30 * time.Millisecond
	// DefaultExpiryTimeout bounds how long expiry polls wait before failing.
	DefaultExpiryTimeout = 2 * time.Second
	// DefaultPollInterval is the tick between expiry-poll attempts.
	DefaultPollInterval = 5 * time.Millisecond
)

// Conformance verifies factory-built caches implement the cache.Cache contract:
// Get/Set round-trip, TTL expiry, SetIfAbsent, Delete, Increment/Decrement,
// Exists, and Close. Each subtest takes a fresh instance from factory so
// cases stay isolated. Expiry waits poll with a context deadline; they never
// synchronize with time.Sleep and never touch the network.
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

	ctx := t.Context()
	c := factory(t)

	if _, err := c.Get(ctx, "missing"); !errors.Is(err, cache.ErrNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}

	var nfErr *cache.NotFoundError
	if _, err := c.Get(ctx, "missing"); !errors.As(err, &nfErr) {
		t.Fatalf("errors.As(err, NotFoundError) = false (err = %T %v)", err, err)
	}

	if nfErr.Key != "missing" {
		t.Errorf("NotFoundError.Key = %q, want missing", nfErr.Key)
	}

	val := []byte("v1")
	if err := c.Set(ctx, "k", val, 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	val[0] = 'X'

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v1" {
		t.Fatalf("Get() = %q, want v1 (stored copy)", got)
	}

	got[0] = 'Y'

	again, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(again) != "v1" {
		t.Fatalf("Get() = %q, want v1 (returned copy)", again)
	}

	if err = c.Set(ctx, "k", []byte("v2"), 0); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}

	got, err = c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v2" {
		t.Errorf("Get() = %q, want v2", got)
	}
}

func conformanceTTLExpiry(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	if err := c.Set(ctx, "k", []byte("v"), DefaultEntryTTL); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, err := c.Get(ctx, "k"); err != nil {
		t.Fatalf("Get() before expiry error = %v", err)
	}

	eventually(t, "key expired from Get", func(ctx context.Context) bool {
		_, err := c.Get(ctx, "k")
		return errors.Is(err, cache.ErrNotFound)
	})

	eventually(t, "exists false after expiry", func(ctx context.Context) bool {
		ok, _ := c.Exists(ctx, "k")
		return !ok
	})

	if err := c.Set(ctx, "keep", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, err := c.Get(ctx, "keep"); err != nil {
		t.Errorf("Get(keep) error = %v, want retained (no ttl)", err)
	}
}

func conformanceSetIfAbsent(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	ok, err := c.SetIfAbsent(ctx, "k", []byte("v1"), 0)
	if err != nil || !ok {
		t.Fatalf("SetIfAbsent() = %v,%v want true,nil", ok, err)
	}

	ok, err = c.SetIfAbsent(ctx, "k", []byte("v2"), 0)
	if err != nil || ok {
		t.Fatalf("SetIfAbsent() second = %v,%v want false,nil", ok, err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v1" {
		t.Errorf("Get() = %q, want v1", got)
	}

	if err = c.Set(ctx, "e", []byte("old"), DefaultEntryTTL); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	eventually(t, "expired key accepted by SetIfAbsent", func(ctx context.Context) bool {
		ok, _ := c.SetIfAbsent(ctx, "e", []byte("new"), 0)
		return ok
	})

	got, err = c.Get(ctx, "e")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "new" {
		t.Errorf("Get() = %q, want new", got)
	}
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	if err := c.Delete(ctx, "missing"); err != nil {
		t.Fatalf("Delete(missing) error = %v, want nil", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || ok {
		t.Errorf("Exists() = %v,%v want false,nil", ok, err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete(again) error = %v, want nil", err)
	}
}

func conformanceCounters(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	if err := c.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	got, err := c.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "1" {
		t.Fatalf("Get() = %q, want 1", got)
	}

	if err = c.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	if err = c.Decrement(ctx, "n"); err != nil {
		t.Fatalf("Decrement() error = %v", err)
	}

	got, err = c.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "1" {
		t.Errorf("Get() = %q, want 1", got)
	}

	if err = c.Decrement(ctx, "fresh"); err != nil {
		t.Fatalf("Decrement() error = %v", err)
	}

	got, err = c.Get(ctx, "fresh")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "-1" {
		t.Errorf("Get() = %q, want -1 (missing base)", got)
	}

	if err = c.Set(ctx, "base", []byte("41"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err = c.Increment(ctx, "base"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	got, err = c.Get(ctx, "base")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "42" {
		t.Errorf("Get() = %q, want 42", got)
	}

	if err = c.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err = c.Increment(ctx, "bad")
	if err == nil {
		t.Fatal("Increment(abc) = nil, want ErrInvalidValue")
	}

	if !errors.Is(err, cache.ErrInvalidValue) {
		t.Errorf("errors.Is(err, ErrInvalidValue) = false (err = %v)", err)
	}

	var invErr *cache.InvalidValueError
	if !errors.As(err, &invErr) {
		t.Fatalf("errors.As(err, InvalidValueError) = false (err = %T %v)", err, err)
	}

	if invErr.Key != "bad" {
		t.Errorf("InvalidValueError.Key = %q, want bad", invErr.Key)
	}
}

func conformanceExists(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	if ok, err := c.Exists(ctx, "missing"); err != nil || ok {
		t.Fatalf("Exists() = %v,%v want false,nil", ok, err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || !ok {
		t.Fatalf("Exists() = %v,%v want true,nil", ok, err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if ok, err := c.Exists(ctx, "k"); err != nil || ok {
		t.Fatalf("Exists() after delete = %v,%v want false,nil", ok, err)
	}

	if err := c.Set(ctx, "e", []byte("v"), DefaultEntryTTL); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	eventually(t, "exists false for expired entry", func(ctx context.Context) bool {
		ok, err := c.Exists(ctx, "e")
		return err == nil && !ok
	})
}

func conformanceClose(t *testing.T, factory func(t *testing.T) cache.Cache) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := c.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Get() err = %v, want ErrClosed", err)
	}

	if err := c.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Set() err = %v, want ErrClosed", err)
	}

	if _, err := c.SetIfAbsent(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("SetIfAbsent() err = %v, want ErrClosed", err)
	}

	if err := c.Delete(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Delete() err = %v, want ErrClosed", err)
	}

	if err := c.Increment(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Increment() err = %v, want ErrClosed", err)
	}

	if err := c.Decrement(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Decrement() err = %v, want ErrClosed", err)
	}

	if _, err := c.Exists(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Exists() err = %v, want ErrClosed", err)
	}
}

// eventually polls cond until true or DefaultExpiryTimeout elapses. Poll
// ticks use a ticker, never time.Sleep, and cond receives a deadline-bound
// context so backend calls share the same deadline.
func eventually(t *testing.T, msg string, cond func(ctx context.Context) bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), DefaultExpiryTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		if cond(ctx) {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("condition not met within %v: %s", DefaultExpiryTimeout, msg)
		case <-ticker.C:
		}
	}
}
