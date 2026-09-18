package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/cache"
)

func mustSetLive(t *testing.T, c cache.Cache, key string, value []byte, ttl time.Duration) {
	t.Helper()

	if err := c.Set(context.Background(), key, value, ttl); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestRedisLive_compareAndDelete(t *testing.T) {
	ctx := context.Background()
	a, _ := newLiveAdapter(t)
	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	mustSetLive(t, a, "k", []byte("holder1"), 0)

	deleted, err := cas.CompareAndDelete(ctx, "k", []byte("holder2"))
	if err != nil || deleted {
		t.Fatalf("CompareAndDelete(mismatch) = %v,%v want false,nil", deleted, err)
	}

	if _, getErr := a.Get(ctx, "k"); getErr != nil {
		t.Fatalf("Get() after mismatch error = %v, want retained", getErr)
	}

	deleted, err = cas.CompareAndDelete(ctx, "k", []byte("holder1"))
	if err != nil || !deleted {
		t.Fatalf("CompareAndDelete(match) = %v,%v want true,nil", deleted, err)
	}

	if _, getErr := a.Get(ctx, "k"); !errors.Is(getErr, cache.ErrNotFound) {
		t.Errorf("Get() after delete err = %v, want ErrNotFound", getErr)
	}

	deleted, err = cas.CompareAndDelete(ctx, "missing", []byte("x"))
	if err != nil || deleted {
		t.Errorf("CompareAndDelete(missing) = %v,%v want false,nil", deleted, err)
	}
}

func TestRedisLive_compareAndExtend(t *testing.T) {
	ctx := context.Background()
	a, s := newLiveAdapter(t)
	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	mustSetLive(t, a, "k", []byte("holder1"), time.Minute)

	extended, err := cas.CompareAndExtend(ctx, "k", []byte("other"), time.Minute)
	if err != nil || extended {
		t.Fatalf("CompareAndExtend(mismatch) = %v,%v want false,nil", extended, err)
	}

	extended, err = cas.CompareAndExtend(ctx, "missing", []byte("x"), time.Minute)
	if err != nil || extended {
		t.Fatalf("CompareAndExtend(missing) = %v,%v want false,nil", extended, err)
	}

	extended, err = cas.CompareAndExtend(ctx, "k", []byte("holder1"), time.Minute)
	if err != nil || !extended {
		t.Fatalf("CompareAndExtend(match) = %v,%v want true,nil", extended, err)
	}

	s.FastForward(30 * time.Second)

	if _, getErr := a.Get(ctx, "k"); getErr != nil {
		t.Errorf("Get() after extend error = %v, want retained", getErr)
	}

	extended, err = cas.CompareAndExtend(ctx, "k", []byte("holder1"), 0)
	if err != nil || !extended {
		t.Fatalf("CompareAndExtend(persist) = %v,%v want true,nil", extended, err)
	}

	if got := s.TTL("k"); got != 0 {
		t.Errorf("TTL() = %v, want 0 (persisted)", got)
	}
}

func TestRedisLive_compareAndExtend_expiryWithoutRenewal(t *testing.T) {
	ctx := context.Background()
	a, s := newLiveAdapter(t)
	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	mustSetLive(t, a, "e", []byte("v"), 500*time.Millisecond)
	s.FastForward(time.Second)

	extended, err := cas.CompareAndExtend(ctx, "e", []byte("v"), time.Minute)
	if err != nil || extended {
		t.Errorf("CompareAndExtend(expired) = %v,%v want false,nil", extended, err)
	}

	deleted, err := cas.CompareAndDelete(ctx, "e", []byte("v"))
	if err != nil || deleted {
		t.Errorf("CompareAndDelete(expired) = %v,%v want false,nil", deleted, err)
	}
}

func TestRedisLive_casClosed_errors(t *testing.T) {
	ctx := context.Background()
	a, _ := newLiveAdapter(t)

	if err := a.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	if _, err := cas.CompareAndDelete(ctx, "k", []byte("v")); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("CompareAndDelete(closed) err = %v, want ErrClosed", err)
	}

	if _, err := cas.CompareAndExtend(ctx, "k", []byte("v"), time.Minute); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("CompareAndExtend(closed) err = %v, want ErrClosed", err)
	}
}

func TestRedisLive_casServerDown_wrapsErrors(t *testing.T) {
	ctx := context.Background()
	a, s := newLiveAdapter(t)
	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	s.Close()

	if _, err := cas.CompareAndDelete(ctx, "k", []byte("v")); err == nil {
		t.Error("CompareAndDelete(down) = nil, want wrapped transport error")
	}

	if _, err := cas.CompareAndExtend(ctx, "k", []byte("v"), time.Minute); err == nil {
		t.Error("CompareAndExtend(down ttl) = nil, want wrapped transport error")
	}

	if _, err := cas.CompareAndExtend(ctx, "k", []byte("v"), 0); err == nil {
		t.Error("CompareAndExtend(down persist) = nil, want wrapped transport error")
	}
}

func TestRedisLive_casNeverStealsSuccessor(t *testing.T) {
	ctx := context.Background()
	a, _ := newLiveAdapter(t)
	cas, ok := a.(cache.CompareAndSwapCache)
	if !ok {
		t.Fatalf("adapter %T does not implement CompareAndSwapCache", a)
	}

	mustSetLive(t, a, "lease", []byte("holder-A"), 0)
	mustSetLive(t, a, "lease2", []byte("holder-A"), time.Minute)

	if err := a.Set(ctx, "lease", []byte("holder-B"), 0); err != nil {
		t.Fatalf("Set(successor) error = %v", err)
	}

	if err := a.Set(ctx, "lease2", []byte("holder-B"), time.Minute); err != nil {
		t.Fatalf("Set(successor2) error = %v", err)
	}

	if deleted, err := cas.CompareAndDelete(ctx, "lease", []byte("holder-A")); err != nil || deleted {
		t.Errorf("stale CompareAndDelete = %v,%v want false,nil", deleted, err)
	}

	if extended, err := cas.CompareAndExtend(ctx, "lease2", []byte("holder-A"), time.Minute); err != nil || extended {
		t.Errorf("stale CompareAndExtend = %v,%v want false,nil", extended, err)
	}

	got, err := a.Get(ctx, "lease")
	if err != nil || string(got) != "holder-B" {
		t.Errorf("Get(lease) = %q,%v want holder-B,nil", got, err)
	}

	got, err = a.Get(ctx, "lease2")
	if err != nil || string(got) != "holder-B" {
		t.Errorf("Get(lease2) = %q,%v want holder-B,nil", got, err)
	}
}
