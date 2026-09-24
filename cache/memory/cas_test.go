package memory

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/cache"
)

func mustSet(t *testing.T, c cache.Cache, key string, value []byte, ttl time.Duration) {
	t.Helper()

	if err := c.Set(t.Context(), key, value, ttl); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestMemoryCompareAndDelete_matchDeletes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("holder1"), 0)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	deleted, err := cas.CompareAndDelete(ctx, "k", []byte("holder1"))
	if err != nil {
		t.Fatalf("CompareAndDelete() error = %v", err)
	}

	if !deleted {
		t.Fatal("CompareAndDelete() = false, want true")
	}

	if _, err := c.Get(ctx, "k"); err == nil {
		t.Error("Get() = nil after delete, want NotFound")
	}
}

func TestMemoryCompareAndDelete_mismatchKeeps(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("holder1"), 0)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	deleted, err := cas.CompareAndDelete(ctx, "k", []byte("holder2"))
	if err != nil {
		t.Fatalf("CompareAndDelete() error = %v", err)
	}

	if deleted {
		t.Fatal("CompareAndDelete() = true for mismatch, want false")
	}

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "holder1" {
		t.Errorf("Get() = %q, want holder1 (successor intact)", got)
	}
}

func TestMemoryCompareAndDelete_missingOrExpiredFalse(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	if deleted, err := cas.CompareAndDelete(ctx, "missing", []byte("x")); err != nil || deleted {
		t.Errorf("CompareAndDelete(missing) = %v,%v want false,nil", deleted, err)
	}

	mustSet(t, c, "e", []byte("v"), 30*time.Millisecond)
	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "e")
		return errors.Is(err, cache.ErrNotFound)
	}, "key expired")

	if deleted, err := cas.CompareAndDelete(ctx, "e", []byte("v")); err != nil || deleted {
		t.Errorf("CompareAndDelete(expired) = %v,%v want false,nil", deleted, err)
	}
}

func TestMemoryCompareAndExtend_matchRenews(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("holder1"), 60*time.Millisecond)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	extended, err := cas.CompareAndExtend(ctx, "k", []byte("holder1"), 5*time.Minute)
	if err != nil {
		t.Fatalf("CompareAndExtend() error = %v", err)
	}

	if !extended {
		t.Fatal("CompareAndExtend() = false, want true")
	}

	// Wait past the original 60ms TTL, polling that the extended entry
	// stays live the whole time.
	start := time.Now()
	waitFor(t, 2*time.Second, func() bool { return time.Since(start) > 80*time.Millisecond }, "past original TTL")

	if _, err := c.Get(ctx, "k"); err != nil {
		t.Errorf("Get() after extend error = %v, want retained", err)
	}
}

func TestMemoryCompareAndExtend_nonPositiveTTLClearsExpiry(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("holder1"), 50*time.Millisecond)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	extended, err := cas.CompareAndExtend(ctx, "k", []byte("holder1"), 0)
	if err != nil || !extended {
		t.Fatalf("CompareAndExtend(persist) = %v,%v want true,nil", extended, err)
	}

	// Expiry cleared: wait past the original 50ms TTL and assert retention.
	start := time.Now()
	waitFor(t, 2*time.Second, func() bool { return time.Since(start) > 80*time.Millisecond }, "past original TTL")

	if _, err := c.Get(ctx, "k"); err != nil {
		t.Errorf("Get() after persist error = %v, want retained (expiry cleared)", err)
	}
}

func TestMemoryCompareAndExtend_mismatchOrExpiredFalse(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("holder1"), 0)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	if extended, err := cas.CompareAndExtend(ctx, "k", []byte("other"), time.Minute); err != nil || extended {
		t.Errorf("CompareAndExtend(mismatch) = %v,%v want false,nil", extended, err)
	}

	if extended, err := cas.CompareAndExtend(ctx, "missing", []byte("x"), time.Minute); err != nil || extended {
		t.Errorf("CompareAndExtend(missing) = %v,%v want false,nil", extended, err)
	}

	mustSet(t, c, "e", []byte("v"), 30*time.Millisecond)
	waitFor(t, 2*time.Second, func() bool {
		_, err := c.Get(ctx, "e")
		return errors.Is(err, cache.ErrNotFound)
	}, "key expired")

	if extended, err := cas.CompareAndExtend(ctx, "e", []byte("v"), time.Minute); err != nil || extended {
		t.Errorf("CompareAndExtend(expired) = %v,%v want false,nil", extended, err)
	}
}

func TestMemoryCompareAndSwap_closedErrors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c, err := New(cache.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	if _, err := cas.CompareAndDelete(ctx, "k", []byte("v")); err == nil {
		t.Error("CompareAndDelete(closed) = nil, want ErrClosed")
	}

	if _, err := cas.CompareAndExtend(ctx, "k", []byte("v"), time.Minute); err == nil {
		t.Error("CompareAndExtend(closed) = nil, want ErrClosed")
	}
}

func TestMemoryCompareAndSwap_concurrentNoSteal(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	c := newTestCache(t, cache.Options{})
	mustSet(t, c, "k", []byte("owner"), 0)

	cas, ok := c.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("cache %T does not implement CompareAndSwapCache", c)
	}

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, _ = cas.CompareAndDelete(ctx, "k", []byte("rival"))
			_, _ = cas.CompareAndExtend(ctx, "k", []byte("rival"), time.Minute)
		}()
	}

	wg.Wait()

	got, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "owner" {
		t.Errorf("Get() = %q, want owner (rivals never stole)", got)
	}

	deleted, err := cas.CompareAndDelete(ctx, "k", []byte("owner"))
	if err != nil || !deleted {
		t.Errorf("CompareAndDelete(owner) = %v,%v want true,nil", deleted, err)
	}
}
