package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/lock"
)

// fakeClient scripts redisClient behavior without a server.
type fakeClient struct {
	setNXFn func(ctx context.Context, key string, value any, exp time.Duration) (bool, error)
	evalFn  func(ctx context.Context, script string, keys []string, args ...any) (int64, error)
}

func (f *fakeClient) SetNX(ctx context.Context, key string, value any, exp time.Duration) *goredis.BoolCmd {
	v, err := f.setNXFn(ctx, key, value, exp)

	return goredis.NewBoolResult(v, err)
}

func (f *fakeClient) Eval(ctx context.Context, script string, keys []string, args ...any) *goredis.Cmd {
	v, err := f.evalFn(ctx, script, keys, args...)

	return goredis.NewCmdResult(v, err)
}

func TestRedactURL_masks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		opts  lock.Options
		want  string
		masks bool
	}{
		{"empty", lock.Options{}, "", false},
		{"plain addr passthrough", lock.Options{Addr: "localhost:6379"}, "localhost:6379", false},
		{"url password masked", lock.Options{URL: "redis://:s3cret@h:6379"}, "redis://:xxxxx@h:6379", true},
		{"url user+password masked", lock.Options{URL: "redis://user:s3cret@h:6379"}, "redis://user:xxxxx@h:6379", true}, //nolint:gosec // test credential
		{"addr fallback masked", lock.Options{Addr: "redis://:s3cret@h:6379"}, "redis://:xxxxx@h:6379", true},
		{"url preferred over addr", lock.Options{URL: "redis://:s3cret@h:6379", Addr: "other:6379"}, "redis://:xxxxx@h:6379", true},
		{"unparsable raw", lock.Options{URL: "redis://[::1"}, "redis://[::1", false},
		{"whitespace trimmed", lock.Options{URL: "  localhost:6379  "}, "localhost:6379", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := redactURL(tc.opts); got != tc.want {
				t.Errorf("redactURL(%+v) = %q, want %q", tc.opts, got, tc.want)
			}

			if strings.Contains(tc.want, "s3cret") {
				t.Errorf("redactURL leaks password: %q", tc.want)
			}

			if tc.masks && !strings.Contains(redactURL(tc.opts), "xxxxx") {
				t.Errorf("redactURL missing xxxxx: %q", redactURL(tc.opts))
			}
		})
	}
}

func TestNewHolderID_ok(t *testing.T) {
	t.Parallel()

	a, err := newHolderID()
	if err != nil {
		t.Fatalf("newHolderID: %v", err)
	}

	b, err := newHolderID()
	if err != nil {
		t.Fatalf("newHolderID: %v", err)
	}

	if a == b || len(a) != 32 {
		t.Errorf("holder ids = %q, %q, want distinct 32-hex", a, b)
	}
}

func TestNewHolderID_randFailure(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom") }
	t.Cleanup(func() { randRead = old })

	if _, err := newHolderID(); err == nil {
		t.Fatal("newHolderID succeeded, want rand error")
	}
}

func TestTryAcquire_emptyKey(t *testing.T) {
	t.Parallel()

	a := &adapter{client: &fakeClient{}, prefix: "lock:", ttl: time.Second}

	if _, ok, err := a.TryAcquire(t.Context(), "", time.Second); err == nil || ok {
		t.Errorf("TryAcquire empty = %v, %v, want error", ok, err)
	}
}

func TestTryAcquire_holderError(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom") }
	t.Cleanup(func() { randRead = old })

	a := &adapter{client: &fakeClient{}, prefix: "lock:", ttl: time.Second}

	if _, _, err := a.TryAcquire(t.Context(), "k", time.Second); err == nil {
		t.Fatal("TryAcquire succeeded, want holder id error")
	}
}

func TestTryAcquire_setNXError(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) {
				return false, errors.New("boom")
			},
		},
		prefix: "lock:",
		ttl:    time.Second,
	}

	if _, _, err := a.TryAcquire(t.Context(), "k", time.Second); err == nil {
		t.Fatal("TryAcquire succeeded, want SetNX error")
	}
}

func TestAcquire_propagatesTryError(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) {
				return false, errors.New("boom")
			},
		},
		prefix:        "lock:",
		ttl:           time.Second,
		retryInterval: time.Millisecond,
	}

	if _, err := a.Acquire(t.Context(), "k", time.Second); err == nil {
		t.Fatal("Acquire succeeded, want SetNX error")
	}
}

func TestAcquire_ctxCancel(t *testing.T) {
	t.Parallel()

	a := &adapter{
		client: &fakeClient{
			setNXFn: func(context.Context, string, any, time.Duration) (bool, error) {
				return false, nil
			},
		},
		prefix:        "lock:",
		ttl:           time.Second,
		retryInterval: time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := a.Acquire(ctx, "k", time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Acquire = %v, want context.Canceled", err)
	}

	if !strings.Contains(err.Error(), `"k"`) {
		t.Errorf("Acquire error = %v, want key context", err)
	}
}

func TestExtend_evalError(t *testing.T) {
	t.Parallel()

	h := &handle{
		a:      &adapter{prefix: "lock:", ttl: time.Second},
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, errors.New("boom") }},
		key:    "k",
	}

	if err := h.Extend(t.Context(), time.Second); err == nil {
		t.Fatal("Extend succeeded, want Eval error")
	}
}

func TestUnlock_evalError(t *testing.T) {
	t.Parallel()

	h := &handle{
		a:      &adapter{prefix: "lock:", ttl: time.Second},
		client: &fakeClient{evalFn: func(context.Context, string, []string, ...any) (int64, error) { return 0, errors.New("boom") }},
		key:    "k",
	}

	if err := h.Unlock(t.Context()); err == nil {
		t.Fatal("Unlock succeeded, want Eval error")
	}
}

func TestClose_error(t *testing.T) {
	old := closeShared
	closeShared = func(*goredis.Client) error { return errors.New("boom") }
	t.Cleanup(func() { closeShared = old })

	a := &adapter{client: &fakeClient{}}

	if err := a.Close(t.Context()); err == nil {
		t.Fatal("Close succeeded, want shared-close error")
	}
}

func TestClose_idempotent(t *testing.T) {
	old := closeShared
	calls := 0
	closeShared = func(*goredis.Client) error { calls++; return nil }
	t.Cleanup(func() { closeShared = old })

	a := &adapter{client: &fakeClient{}}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if calls != 1 {
		t.Errorf("shared close calls = %d, want 1", calls)
	}
}

func TestRegisterAndOpen(t *testing.T) {
	if err := lock.Register(lock.Redis, New); err != nil && !errors.Is(err, lock.ErrDuplicate) {
		t.Fatalf("Register: %v", err)
	}

	if _, err := lock.Open(lock.Redis, lock.Options{Addr: "127.0.0.1:1"}); err == nil {
		t.Fatal("Open with refused addr succeeded, want ping error")
	}
}
