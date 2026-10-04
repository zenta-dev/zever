package memory

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

func stubCache(t *testing.T, opts cache.Options) cache.Cache {
	t.Helper()

	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = c.Close(t.Context()) })

	return c
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}

	t.Fatalf("condition not met within %v: %s", d, msg)
}

func TestMemorySetGet_roundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v" {
		t.Errorf("Get() = %q, want v", got)
	}
}

func TestMemoryGet_copiesValue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	val := []byte("v")
	if err := c.Set(ctx, "k", val, 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	val[0] = 'X'

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v" {
		t.Errorf("Get() = %q, want v (stored copy)", got)
	}

	got[0] = 'Y'

	again, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(again) != "v" {
		t.Errorf("Get() = %q, want v (returned copy)", again)
	}
}

func TestMemorySet_overwrites(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "k", []byte("v1"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.Set(ctx, "k", []byte("v2"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v2" {
		t.Errorf("Get() = %q, want v2", got)
	}
}

func TestMemorySetIfAbsent_semantics(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

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

	if setErr := c.Set(ctx, "e", []byte("old"), 30*time.Millisecond); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}

	waitFor(t, 2*time.Second, func() bool {
		ok, _ := c.SetIfAbsent(ctx, "e", []byte("new"), 0)
		return ok
	}, "expired key accepted by SetIfAbsent")

	got, err = c.Get(ctx, "e")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "new" {
		t.Errorf("Get() = %q, want new", got)
	}
}

func TestMemoryDelete_missingOrPresent_nilError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Delete(ctx, "missing"); err != nil {
		t.Errorf("Delete(missing) error = %v, want nil", err)
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
}

func TestMemoryTTL_expiry(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "k", []byte("v"), 40*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "k")
		return errors.Is(err, cache.ErrNotFound)
	}, "key expired from Get")

	waitFor(t, 2*time.Second, func() bool {
		ok, _ := c.Exists(ctx, "k")
		return !ok
	}, "exists false after expiry")

	if err := c.Set(ctx, "keep", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// keep has no TTL: expiry of "k" above already proves time passed,
	// so it must still be present without any further wait.
	if _, err := c.Get(ctx, "keep"); err != nil {
		t.Errorf("Get(keep) error = %v, want retained (no ttl)", err)
	}
}

func TestMemoryJanitor_sweepsExpired(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{SweepInterval: 20 * time.Millisecond})

	if err := c.Set(ctx, "k", []byte("v"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		_, err := c.Get(ctx, "k")
		return errors.Is(err, cache.ErrNotFound)
	}, "janitor swept expired key")
}

func TestMemoryEviction_lruOldestGoesFirst(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{MaxEntries: 3})

	for _, k := range []string{"a", "b", "c"} {
		if err := c.Set(ctx, k, []byte(k), 0); err != nil {
			t.Fatalf("Set(%s) error = %v", k, err)
		}
	}

	if _, err := c.Get(ctx, "a"); err != nil {
		t.Fatalf("Get(a) error = %v", err)
	}

	if err := c.Set(ctx, "d", []byte("d"), 0); err != nil {
		t.Fatalf("Set(d) error = %v", err)
	}

	if _, err := c.Get(ctx, "b"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get(b) err = %v, want NotFound (evicted)", err)
	}

	for _, k := range []string{"a", "c", "d"} {
		if _, err := c.Get(ctx, k); err != nil {
			t.Errorf("Get(%s) err = %v, want retained", k, err)
		}
	}
}

func TestMemoryIncrement_missingStartsAtOne(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	got, err := c.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "1" {
		t.Errorf("Get() = %q, want 1", got)
	}

	if err := c.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	if err := c.Decrement(ctx, "n"); err != nil {
		t.Fatalf("Decrement() error = %v", err)
	}

	if err := c.Decrement(ctx, "n"); err != nil {
		t.Fatalf("Decrement() error = %v", err)
	}

	got, getErr := c.Get(ctx, "n")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}

	if string(got) != "0" {
		t.Errorf("Get() = %q, want 0", got)
	}
}

func TestMemoryIncrement_preservesValue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "k", []byte("41"), 5*time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.Increment(ctx, "k"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "42" {
		t.Errorf("Get() = %q, want 42", got)
	}
}

func TestMemoryExists_expiredEntry_removed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "k", []byte("v"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Exists first: entry still present but expired, exercising the
	// lazy-expiry path (default sweep interval is a minute, so the
	// janitor cannot race us here).
	waitFor(t, 2*time.Second, func() bool {
		ok, err := c.Exists(ctx, "k")
		return err == nil && !ok
	}, "exists false for expired entry")

	if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
}

func TestMemoryIncrement_expiredKey_restartsAtOne(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "n", []byte("41"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "n")
		return errors.Is(err, cache.ErrNotFound)
	}, "key expired")

	if err := c.Set(ctx, "n", []byte("41"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "n")
		return errors.Is(err, cache.ErrNotFound)
	}, "re-set key expired")

	if err := c.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	got, err := c.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "1" {
		t.Errorf("Get() = %q, want 1 (expired base discarded)", got)
	}
}

func TestMemorySweep_removesOnlyExpired(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	a, ok := c.(*memoryAdapter)
	if !ok {
		t.Fatalf("New() type = %T, want *memoryAdapter", c)
	}

	if err := c.Set(ctx, "old", []byte("v"), 30*time.Millisecond); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.Set(ctx, "keep", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Wait past the 30ms TTL without touching "old" (Get/Exists would
	// lazily purge it and hide the sweep path), then sweep explicitly.
	start := time.Now()
	waitFor(t, 2*time.Second, func() bool { return time.Since(start) > 50*time.Millisecond }, "past TTL")
	a.sweep()

	if _, err := c.Get(ctx, "old"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get(old) err = %v, want NotFound (swept)", err)
	}

	if _, err := c.Get(ctx, "keep"); err != nil {
		t.Errorf("Get(keep) err = %v, want retained", err)
	}

	a.sweep()
}

func TestMemorySetIfAbsent_withTTL_expires(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	ok, err := c.SetIfAbsent(ctx, "k", []byte("v"), 40*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("SetIfAbsent() = %v,%v want true,nil", ok, err)
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v" {
		t.Errorf("Get() = %q, want v", got)
	}

	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "k")
		return errors.Is(err, cache.ErrNotFound)
	}, "ttl entry from SetIfAbsent expired")
}

func TestMemoryIncrement_nonInteger_invalidValue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	if err := c.Set(ctx, "counter", []byte("abc"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err := c.Increment(ctx, "counter")
	if err == nil {
		t.Fatal("Increment(abc) = nil, want ErrInvalidValue")
	}

	if !errors.Is(err, cache.ErrInvalidValue) {
		t.Errorf("errors.Is(err, ErrInvalidValue) = false (err = %v)", err)
	}

	var invErr cache.InvalidValueError
	if !errors.As(err, &invErr) {
		t.Fatalf("errors.As(err, InvalidValueError) = false (err = %T %v)", err, err)
	}

	if invErr.Key != "counter" {
		t.Errorf("InvalidValueError.Key = %q, want counter", invErr.Key)
	}
}

func TestMemoryClose_presetClosed_returnsNil(t *testing.T) {
	t.Parallel()

	c := stubCache(t, cache.Options{})

	a, ok := c.(*memoryAdapter)
	if !ok {
		t.Fatalf("New() type = %T, want *memoryAdapter", c)
	}

	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()

	if err := c.Close(t.Context()); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestMemorySweep_racyBranches(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	a, ok := c.(*memoryAdapter)
	if !ok {
		t.Fatalf("New() type = %T, want *memoryAdapter", c)
	}

	var wg sync.WaitGroup

	for i := range 1500 {
		key := fmt.Sprintf("race-%d", i)

		if err := c.Set(ctx, key, []byte("v"), time.Nanosecond); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		wg.Add(2)
		go func() { defer wg.Done(); a.sweep() }()
		go func() { defer wg.Done(); a.sweep() }()

		_ = c.Delete(ctx, key)
		_ = c.Set(ctx, key, []byte("v"), 0)
	}

	wg.Wait()
}

func TestMemoryClosed_allOpsFail(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c, err := New(cache.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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

func TestMemoryConcurrent_mixedOps_safe(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	var wg sync.WaitGroup

	for w := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range 100 {
				key := fmt.Sprintf("w%d-k%d", w, i%10)

				_ = c.Set(ctx, key, []byte("v"), 0)
				_, _ = c.Get(ctx, key)
				_, _ = c.Exists(ctx, key)
				_, _ = c.SetIfAbsent(ctx, key, []byte("v"), 0)

				if i%10 == 0 {
					_ = c.Delete(ctx, key)
				}
			}
		}()
	}

	wg.Wait()
}

func TestMemoryConcurrent_increment_safe(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := stubCache(t, cache.Options{})

	const workers = 10
	const perWorker = 50

	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range perWorker {
				if err := c.Increment(ctx, "counter"); err != nil {
					t.Errorf("Increment() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	got, err := c.Get(ctx, "counter")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "500" {
		t.Errorf("Get() = %q, want 500", got)
	}
}
