package redis

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/cache"
)

// newLiveAdapter starts an in-process miniredis server (loopback only,
// no external network) and opens a cache adapter against it.
// Tests here are sequential: New threads through the shared zredis
// singleton, so t.Parallel is forbidden in this file.
func newLiveAdapter(t *testing.T) (cache.Cache, *miniredis.Miniredis) {
	t.Helper()

	s := miniredis.RunT(t)

	a, err := New(cache.Options{Addr: s.Addr()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() {
		_ = a.Close(t.Context())
	})

	return a, s
}

func TestRedisLive_roundTrip(t *testing.T) {
	ctx := t.Context()
	a, _ := newLiveAdapter(t)

	if err := a.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := a.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v" {
		t.Errorf("Get() = %q, want v", got)
	}

	ok, err := a.Exists(ctx, "k")
	if err != nil || !ok {
		t.Errorf("Exists() = %v,%v want true,nil", ok, err)
	}

	ok, err = a.Exists(ctx, "missing")
	if err != nil || ok {
		t.Errorf("Exists(missing) = %v,%v want false,nil", ok, err)
	}

	if err := a.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if err := a.Delete(ctx, "missing"); err != nil {
		t.Errorf("Delete(missing) error = %v, want nil", err)
	}

	if _, err := a.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}

	var nfErr *cache.NotFoundError
	if _, err := a.Get(ctx, "k"); !errors.As(err, &nfErr) {
		t.Errorf("errors.As(err, NotFoundError) = false (err = %T %v)", err, err)
	} else if nfErr.Key != "k" {
		t.Errorf("NotFoundError.Key = %q, want k", nfErr.Key)
	}
}

func TestRedisLive_ttlExpiry(t *testing.T) {
	ctx := t.Context()
	a, s := newLiveAdapter(t)

	if err := a.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	s.FastForward(2 * time.Minute)

	if _, err := a.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
}

func TestRedisLive_setIfAbsent(t *testing.T) {
	ctx := t.Context()
	a, _ := newLiveAdapter(t)

	ok, err := a.SetIfAbsent(ctx, "k", []byte("v1"), 0)
	if err != nil || !ok {
		t.Fatalf("SetIfAbsent() = %v,%v want true,nil", ok, err)
	}

	ok, err = a.SetIfAbsent(ctx, "k", []byte("v2"), 0)
	if err != nil || ok {
		t.Fatalf("SetIfAbsent() second = %v,%v want false,nil", ok, err)
	}

	got, err := a.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "v1" {
		t.Errorf("Get() = %q, want v1", got)
	}
}

func TestRedisLive_counters(t *testing.T) {
	ctx := t.Context()
	a, _ := newLiveAdapter(t)

	if err := a.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	if err := a.Increment(ctx, "n"); err != nil {
		t.Fatalf("Increment() error = %v", err)
	}

	if err := a.Decrement(ctx, "n"); err != nil {
		t.Fatalf("Decrement() error = %v", err)
	}

	got, err := a.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "1" {
		t.Errorf("Get() = %q, want 1", got)
	}

	if err := a.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := a.Increment(ctx, "bad"); err == nil {
		t.Error("Increment(abc) = nil, want error")
	}
}

func TestRedisLive_passwordAuth(t *testing.T) {
	ctx := t.Context()

	s := miniredis.RunT(t)
	s.RequireAuth("pw")

	a, err := New(cache.Options{Addr: s.Addr(), Password: "pw"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = a.Close(t.Context()) })

	if err := a.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, err := a.Get(ctx, "k"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestRedisLive_closeDelegatesAndIdempotent(t *testing.T) {
	ctx := t.Context()
	a, _ := newLiveAdapter(t)

	if err := a.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := a.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := a.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Get() err = %v, want ErrClosed", err)
	}
}

func TestRedisLive_serverDown_opsWrapErrors(t *testing.T) {
	ctx := t.Context()
	a, s := newLiveAdapter(t)

	s.Close()

	if _, err := a.Get(ctx, "k"); err == nil || errors.Is(err, cache.ErrClosed) || errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get() err = %v, want wrapped transport error", err)
	}

	if err := a.Set(ctx, "k", []byte("v"), 0); err == nil {
		t.Error("Set() = nil, want wrapped transport error")
	}

	if _, err := a.SetIfAbsent(ctx, "k", []byte("v"), 0); err == nil {
		t.Error("SetIfAbsent() = nil, want wrapped transport error")
	}

	if err := a.Delete(ctx, "k"); err == nil {
		t.Error("Delete() = nil, want wrapped transport error")
	}

	if err := a.Increment(ctx, "k"); err == nil {
		t.Error("Increment() = nil, want wrapped transport error")
	}

	if err := a.Decrement(ctx, "k"); err == nil {
		t.Error("Decrement() = nil, want wrapped transport error")
	}

	if _, err := a.Exists(ctx, "k"); err == nil {
		t.Error("Exists() = nil, want wrapped transport error")
	}
}

func TestRedisLive_pingFailure_redactsPassword(t *testing.T) {
	s := miniredis.RunT(t)
	dead := s.Addr()
	s.Close()

	_, err := New(cache.Options{Addr: "redis://:s3cret@" + dead})
	if err == nil {
		t.Fatal("New() = nil, want ping error")
	}

	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks password: %v", err)
	}

	if !strings.Contains(err.Error(), "xxxxx") {
		t.Errorf("error missing redacted password: %v", err)
	}
}
