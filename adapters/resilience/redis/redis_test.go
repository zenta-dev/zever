package redis_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/adapters/resilience/redis"
	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/core/resilience/resiliencetest"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// testServer starts a per-test miniredis instance, auto-closed via t.Cleanup.
func testServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	return miniredis.RunT(t)
}

// optionsFor builds Options pointed at an already-running server.
func optionsFor(s *miniredis.Miniredis) redis.Options {
	return redis.Options{
		Options: resilience.Options{
			Redis: resilience.RedisOptions{
				Options: redisopt.Options{
					ConnectOptions: redisopt.ConnectOptions{Addr: s.Addr()},
				},
			},
		},
	}
}

func newTestManager(t *testing.T, opts redis.Options) resilience.Manager {
	t.Helper()

	m, err := redis.New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

// TestRedisConformance proves the redis adapter honors the resilience
// contract via the shared conformance kit. Timeout, retry, and bulkhead run
// process-local; only breaker state is distributed.
func TestRedisConformance(t *testing.T) {
	s := testServer(t)

	resiliencetest.Conformance(t, func(t *testing.T, opts resilience.Options) resilience.Manager {
		t.Helper()

		opts.Redis.Addr = s.Addr()

		m, err := redis.New(redis.Options{Options: opts})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = m.Close() })

		return m
	})
}

// TestRedis_sharedBreakerState proves two independently constructed managers
// on the same Redis and prefix share circuit-breaker state: tripping the
// breaker in A opens B's guard, and B recovers through the half-open probe.
func TestRedis_sharedBreakerState(t *testing.T) {
	t.Parallel()

	s := testServer(t)

	breaker := resilience.BreakerOptions{
		Enabled:      true,
		MinRequests:  2,
		FailureRatio: 0.5,
		Timeout:      200 * time.Millisecond,
	}

	optsA := optionsFor(s)
	optsA.Breaker = breaker

	optsB := optionsFor(s)
	optsB.Breaker = breaker

	a := newTestManager(t, optsA)
	b := newTestManager(t, optsB)

	ga, err := a.Guard("dep")
	if err != nil {
		t.Fatalf("A Guard() error = %v", err)
	}

	gb, err := b.Guard("dep")
	if err != nil {
		t.Fatalf("B Guard() error = %v", err)
	}

	boom := errors.New("redis: boom")

	for i := 0; i < 2; i++ {
		if err := ga.Execute(t.Context(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("A Execute(fail) error = %v, want boom", err)
		}
	}

	if ga.State() != resilience.StateOpen {
		t.Fatalf("A State() = %s, want open", ga.State())
	}

	// B never saw a failure but shares the breaker state through Redis.
	if gb.State() != resilience.StateOpen {
		t.Fatalf("B State() = %s, want open (shared breaker state)", gb.State())
	}

	if err := gb.Execute(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, resilience.ErrOpenState) {
		t.Fatalf("B Execute(open) error = %v, want ErrOpenState", err)
	}

	// Cross the open timeout so the shared breaker half-opens; this is the
	// feature under test, not a synchronization primitive.
	time.Sleep(300 * time.Millisecond)

	if err := gb.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("B half-open probe error = %v, want nil", err)
	}

	if gb.State() != resilience.StateClosed {
		t.Fatalf("B State() after probe = %s, want closed", gb.State())
	}
}

// TestRedis_prefixIsolation proves managers with different prefixes keep
// isolated breaker state on the same Redis.
func TestRedis_prefixIsolation(t *testing.T) {
	t.Parallel()

	s := testServer(t)

	optsA := optionsFor(s)
	optsA.Redis.Prefix = "pfxA"
	optsA.Breaker = resilience.BreakerOptions{
		Enabled:      true,
		MinRequests:  1,
		FailureRatio: 0.1,
		Timeout:      time.Second,
	}

	optsB := optionsFor(s)
	optsB.Redis.Prefix = "pfxB"
	optsB.Breaker = optsA.Breaker

	a := newTestManager(t, optsA)
	b := newTestManager(t, optsB)

	ga, err := a.Guard("dep")
	if err != nil {
		t.Fatalf("A Guard() error = %v", err)
	}

	gb, err := b.Guard("dep")
	if err != nil {
		t.Fatalf("B Guard() error = %v", err)
	}

	boom := errors.New("redis: boom")

	if err := ga.Execute(t.Context(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("A Execute(fail) error = %v, want boom", err)
	}

	if ga.State() != resilience.StateOpen {
		t.Fatalf("A State() = %s, want open", ga.State())
	}

	if gb.State() != resilience.StateClosed {
		t.Fatalf("B State() = %s, want closed (isolated prefix)", gb.State())
	}

	if err := gb.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("B Execute() error = %v, want nil", err)
	}
}

// TestRedis_closeIdempotent proves Manager.Close is idempotent and closes the
// shared Redis client exactly once.
func TestRedis_closeIdempotent(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if _, err := m.Guard("after-close"); !errors.Is(err, resilience.ErrClosed) {
		t.Fatalf("Guard(after close) error = %v, want ErrClosed", err)
	}
}

// TestRedis_invalidAddr proves New fails closed when Redis is unreachable or
// the address is malformed.
func TestRedis_invalidAddr(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*redis.Options){
		"malformed": func(o *redis.Options) { o.Redis.Addr = "://bad" },
		"unreachable": func(o *redis.Options) {
			o.Redis.Addr = "127.0.0.1:1"
		},
	}

	for name, mutate := range cases {
		opts := optionsFor(testServer(t))
		mutate(&opts)

		if _, err := redis.New(opts); err == nil {
			t.Errorf("New(%s) error = nil, want error", name)
		}
	}
}

// TestRedis_invalidPrefix proves New fails closed on a prefix that is not a
// valid Redis key prefix.
func TestRedis_invalidPrefix(t *testing.T) {
	t.Parallel()

	opts := optionsFor(testServer(t))
	opts.Redis.Prefix = "has space"

	if _, err := redis.New(opts); err == nil {
		t.Fatal("New(bad prefix) error = nil, want error")
	}
}

// TestRegister_thenOpen proves the adapter wires into the battery registry
// under the redis adapter name.
func TestRegister_thenOpen(t *testing.T) {
	t.Parallel()

	s := testServer(t)

	redis.Register()

	m, err := resilience.Open(resilience.Redis, resilience.Options{
		Redis: resilience.RedisOptions{
			Options: redisopt.Options{
				ConnectOptions: redisopt.ConnectOptions{Addr: s.Addr()},
			},
		},
	})
	if err != nil {
		t.Fatalf("Open(Redis) error = %v", err)
	}

	defer func() { _ = m.Close() }()

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

// TestNew_invalidOptions proves New rejects invalid battery options.
func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	opts := optionsFor(testServer(t))
	opts.Timeout = -1

	if _, err := redis.New(opts); !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("New(invalid) error = %v, want ErrInvalidOptions", err)
	}
}

// TestGuard_emptyName proves Guard rejects an empty name.
func TestGuard_emptyName(t *testing.T) {
	t.Parallel()

	m := newTestManager(t, optionsFor(testServer(t)))

	if _, err := m.Guard(""); !errors.Is(err, resilience.ErrInvalidOptions) {
		t.Fatalf("Guard(empty) error = %v, want ErrInvalidOptions", err)
	}
}

// TestOnStateChange proves state transitions are reported for the
// distributed breaker.
func TestOnStateChange(t *testing.T) {
	t.Parallel()

	var (
		froms []resilience.State
		tos   []resilience.State
	)

	opts := optionsFor(testServer(t))
	opts.Breaker = resilience.BreakerOptions{
		Enabled:      true,
		MinRequests:  1,
		FailureRatio: 0.1,
		Timeout:      time.Second,
	}
	opts.OnStateChange = func(_ string, from, to resilience.State) {
		froms = append(froms, from)
		tos = append(tos, to)
	}

	m := newTestManager(t, opts)

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard() error = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return errors.New("boom") }); err == nil {
		t.Fatal("Execute(fail) error = nil, want failure")
	}

	if len(tos) != 1 {
		t.Fatalf("state changes = %d, want 1", len(tos))
	}

	if froms[0] != resilience.StateClosed || tos[0] != resilience.StateOpen {
		t.Fatalf("transition = %s->%s, want closed->open", froms[0], tos[0])
	}
}
