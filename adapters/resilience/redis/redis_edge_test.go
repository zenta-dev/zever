package redis_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/resilience/redis"
	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

// TestRedisEdge_invalidOptions proves New rejects every invalid battery
// option value before touching Redis.
func TestRedisEdge_invalidOptions(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*redis.Options){
		"timeout":             func(o *redis.Options) { o.Timeout = -1 },
		"retry_base":          func(o *redis.Options) { o.Retry.BaseDelay = -1 },
		"retry_max":           func(o *redis.Options) { o.Retry.MaxDelay = -1 },
		"retry_attempts":      func(o *redis.Options) { o.Retry.MaxAttempts = -1 },
		"retry_jitter":        func(o *redis.Options) { o.Retry.Jitter = 2 },
		"breaker_ratio":       func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, FailureRatio: 2} },
		"breaker_min":         func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, MinRequests: -1} },
		"breaker_consecutive": func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, ConsecutiveFailures: -1} },
		"breaker_interval":    func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, Interval: -1} },
		"breaker_bucket":      func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, BucketPeriod: -1} },
		"breaker_timeout":     func(o *redis.Options) { o.Breaker = resilience.BreakerOptions{Enabled: true, Timeout: -1} },
		"bulkhead_concurrent": func(o *redis.Options) { o.Bulkhead.MaxConcurrent = -1 },
		"bulkhead_queue":      func(o *redis.Options) { o.Bulkhead.MaxQueue = -1 },
		"bulkhead_wait":       func(o *redis.Options) { o.Bulkhead.MaxWait = -1 },
	}

	for name, mutate := range cases {
		opts := optionsFor(testServer(t))
		mutate(&opts)

		if _, err := redis.New(opts); !errors.Is(err, resilience.ErrInvalidOptions) {
			t.Errorf("New(%s) error = %v, want ErrInvalidOptions", name, err)
		}
	}
}

// TestRedisEdge_invalidOptionsAs proves invalid options carry a structured
// InvalidOptionsError for errors.As callers.
func TestRedisEdge_invalidOptionsAs(t *testing.T) {
	t.Parallel()

	opts := optionsFor(testServer(t))
	opts.Timeout = -1

	_, err := redis.New(opts)

	var ioe resilience.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("New(invalid) error type = %T, want InvalidOptionsError", err)
	}
}

// TestRedisEdge_guardCachedAndName proves repeated Guard calls return the
// cached guard and Name reports the dependency name.
func TestRedisEdge_guardCachedAndName(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	first, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	second, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard(same) error = %v", err)
	}

	if first != second {
		t.Error("Guard(same) returned a different Guard, want the cached one")
	}

	if first.Name() != "dep" {
		t.Errorf("Name() = %q, want dep", first.Name())
	}

	if first.State() != resilience.StateClosed {
		t.Errorf("State() = %s, want closed without breaker", first.State())
	}
}

// TestRedisEdge_executeAfterGuardClose proves Execute fails closed once the
// guard itself is closed, and guard Close is idempotent.
func TestRedisEdge_executeAfterGuardClose(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrClosed) {
		t.Fatalf("Execute(after guard close) error = %v, want ErrClosed", err)
	}
}

// TestRedisEdge_executeDeadlineExceeded proves a pre-expired context surfaces
// the deadline error instead of running fn.
func TestRedisEdge_executeDeadlineExceeded(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	called := false
	err = g.Execute(ctx, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute(expired) error = %v, want DeadlineExceeded", err)
	}

	if called {
		t.Error("Execute(expired) ran fn, want fn skipped")
	}
}

// TestRedisEdge_retryExhausted proves the retry path attempts exactly
// MaxAttempts times and wraps the last failure.
func TestRedisEdge_retryExhausted(t *testing.T) {
	t.Parallel()

	opts := optionsFor(testServer(t))
	opts.Retry = retry.Policy{MaxAttempts: 3}

	m := newTestManager(t, opts)

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	sentinel := errors.New("redis-edge: boom")
	calls := 0

	err = g.Execute(t.Context(), func(context.Context) error { calls++; return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Execute(exhausted) error = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Fatalf("Execute(exhausted) error = %q, want retry exhausted mention", err.Error())
	}

	if calls != 3 {
		t.Errorf("fn calls = %d, want 3", calls)
	}
}

// TestRedisEdge_retrySkipsCancellation proves caller cancellation short-circuits
// the retry loop with a single attempt and no OnRetry callback.
func TestRedisEdge_retrySkipsCancellation(t *testing.T) {
	t.Parallel()

	retried := false
	opts := optionsFor(testServer(t))
	opts.Retry = retry.Policy{
		MaxAttempts: 3,
		OnRetry:     func(int, error) { retried = true },
	}

	m := newTestManager(t, opts)

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	calls := 0
	err = g.Execute(t.Context(), func(context.Context) error { calls++; return context.Canceled })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) error = %v, want context.Canceled", err)
	}

	if calls != 1 {
		t.Errorf("fn calls on cancellation = %d, want 1 (no retry)", calls)
	}

	if retried {
		t.Error("OnRetry fired on cancellation, want no retry callback")
	}
}

// TestRedisEdge_parentCancelNotTimeout proves cancelling the caller context
// surfaces cancellation rather than misreporting a timeout.
func TestRedisEdge_parentCancelNotTimeout(t *testing.T) {
	t.Parallel()

	opts := optionsFor(testServer(t))
	opts.Timeout = 5 * time.Second

	m := newTestManager(t, opts)

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})

	go func() {
		<-started
		cancel()
	}()

	err = g.Execute(ctx, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(parent cancel) error = %v, want context.Canceled", err)
	}

	if errors.Is(err, resilience.ErrTimeout) {
		t.Fatalf("Execute(parent cancel) error = %v, must not be ErrTimeout", err)
	}
}

// TestRedisEdge_concurrentExecute proves a guard is safe for concurrent Execute.
func TestRedisEdge_concurrentExecute(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	const n = 16

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
				t.Errorf("Execute() error = %v", err)
			}
		}()
	}

	wg.Wait()
}

// TestRedisEdge_concurrentGuard proves concurrent Guard calls for one name
// converge on the cached guard.
func TestRedisEdge_concurrentGuard(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	const n = 16

	guards := make([]resilience.Guard, n)

	var wg sync.WaitGroup

	wg.Add(n)

	for i := range n {
		go func() {
			defer wg.Done()

			g, err := m.Guard("dep")
			if err != nil {
				t.Errorf("Guard() error = %v", err)
				return
			}

			guards[i] = g
		}()
	}

	wg.Wait()

	for i := 1; i < n; i++ {
		if guards[i] == nil || guards[0] == nil {
			t.Fatalf("guard[%d] is nil after concurrent Guard", i)
		}

		if guards[i] != guards[0] {
			t.Fatalf("guard[%d] != guard[0], want cached Guard", i)
		}
	}
}

// TestRedisEdge_doubleRegister proves Register is safe to call twice and the
// adapter still opens under the redis name.
func TestRedisEdge_doubleRegister(t *testing.T) {
	t.Parallel()

	s := testServer(t)

	redis.Register()
	redis.Register()

	m, err := resilience.Open(resilience.Redis, resilience.Options{
		Redis: resilience.RedisOptions{
			Options: optionsFor(s).Redis.Options,
		},
	})
	if err != nil {
		t.Fatalf("Open(Redis) error = %v", err)
	}

	defer func() { _ = m.Close() }()

	if _, err := m.Guard("dep"); err != nil {
		t.Fatalf("Guard() error = %v", err)
	}
}
