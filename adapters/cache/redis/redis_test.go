package redis

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

func TestRedactURL_masksCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   cache.Options
		want string
	}{
		{name: "empty", in: cache.Options{}, want: ""},
		{name: "plain addr passthrough", in: cache.Options{Addr: "localhost:6379"}, want: "localhost:6379"},
		{name: "url password masked", in: cache.Options{URL: "redis://:s3cret@h:6379"}, want: "redis://:xxxxx@h:6379"},
		{name: "url user and password masked", in: cache.Options{URL: "redis://bob:s3cret@h:6379"}, want: "redis://bob:xxxxx@h:6379"}, //nolint:gosec // fixture credential for redaction test
		{name: "addr fallback masked", in: cache.Options{Addr: "redis://:s3cret@h:6379"}, want: "redis://:xxxxx@h:6379"},
		{name: "url preferred over addr", in: cache.Options{URL: "redis://h1:6379", Addr: "h2:6379"}, want: "redis://h1:6379"},
		{name: "unparsable returned raw", in: cache.Options{Addr: "redis://[::1"}, want: "redis://[::1"},
		{name: "whitespace trimmed", in: cache.Options{Addr: "  localhost:6379 "}, want: "localhost:6379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := redactURL(tt.in); got != tt.want {
				t.Errorf("redactURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func closedAdapter() *redisAdapter {
	a := &redisAdapter{}
	a.closed.Store(true)

	return a
}

func TestRedisClosed_allOpsReturnErrClosed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	a := closedAdapter()

	if _, err := a.Get(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Get() err = %v, want ErrClosed", err)
	}

	if err := a.Set(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Set() err = %v, want ErrClosed", err)
	}

	if _, err := a.SetIfAbsent(ctx, "k", []byte("v"), 0); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("SetIfAbsent() err = %v, want ErrClosed", err)
	}

	if err := a.Delete(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Delete() err = %v, want ErrClosed", err)
	}

	if err := a.Increment(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Increment() err = %v, want ErrClosed", err)
	}

	if err := a.Decrement(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Decrement() err = %v, want ErrClosed", err)
	}

	if _, err := a.Exists(ctx, "k"); !errors.Is(err, cache.ErrClosed) {
		t.Errorf("Exists() err = %v, want ErrClosed", err)
	}

	if err := a.Close(ctx); err != nil {
		t.Errorf("Close() err = %v, want nil (idempotent)", err)
	}
}

func TestRedisNew_invalidAddr_failsBeforeDial(t *testing.T) {
	_, err := New(cache.Options{Addr: "redis://"})
	if err == nil {
		t.Fatal("New() = nil, want error")
	}
}
