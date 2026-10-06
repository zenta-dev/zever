package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"
	gobreakerredis "github.com/sony/gobreaker/v2/redis"
	"golang.org/x/sync/semaphore"

	"github.com/zenta-dev/zever/core/resilience"
	redisclient "github.com/zenta-dev/zever/shared/redisclient"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
	"github.com/zenta-dev/zever/shared/retry"
)

var (
	_ resilience.Manager = (*manager)(nil)
	_ resilience.Guard   = (*guard)(nil)
)

// DefaultPingTimeout bounds the startup connectivity check.
const DefaultPingTimeout = 3 * time.Second

// manager owns the lazily created per-dependency guards for one adapter
// instance. Breaker state is shared through Redis; timeout, retry, and
// bulkhead stay process-local.
type manager struct {
	mu      sync.Mutex
	guards  map[string]*guard
	opts    resilience.Options
	store   gobreaker.SharedDataStore
	release func() error
	prefix  string
	closed  bool
}

// New creates a Redis-backed resilience Manager from opts. The circuit
// breaker state is distributed through Redis so independent managers sharing
// the same addr and prefix trip together; timeout, retry, and bulkhead remain
// process-local because only breaker state is meaningful to share. It fails
// closed when Redis is unreachable.
func New(opts Options) (resilience.Manager, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	prefix := strings.TrimSpace(opts.Redis.Prefix)
	if prefix == "" {
		prefix = resilience.DefaultRedisPrefix
	}

	if err := redisopt.ValidatePrefix(prefix); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	if err := redisopt.ValidateAddr(opts.Redis.Addr); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	client, release, err := redisclient.Shared(opts.Redis.Options)
	if err != nil {
		return nil, fmt.Errorf("redis: connect %q: %w", redisopt.RedactAddr(opts.Redis.Addr), err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = release()

		return nil, fmt.Errorf("redis: ping %q: %w", redisopt.RedactAddr(opts.Redis.Addr), err)
	}

	return &manager{
		guards:  make(map[string]*guard),
		opts:    opts.Options,
		store:   gobreakerredis.NewStoreFromClient(client),
		release: release,
		prefix:  prefix,
	}, nil
}

// Guard returns the named guard, lazily creating it from the template Options.
func (m *manager) Guard(name string) (resilience.Guard, error) {
	if name == "" {
		return nil, resilience.InvalidOptionsError{Reason: "guard name must not be empty"}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, resilience.ErrClosed
	}

	if g, ok := m.guards[name]; ok {
		return g, nil
	}

	g, err := newGuard(name, m.opts, m.prefix, m.store)
	if err != nil {
		return nil, fmt.Errorf("redis: guard %q: %w", name, err)
	}

	m.guards[name] = g

	return g, nil
}

// Close closes every guard, releases the shared Redis client, and is
// idempotent.
func (m *manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	var errs []error

	for _, g := range m.guards {
		errs = append(errs, g.Close())
	}

	m.guards = nil

	if m.release != nil {
		errs = append(errs, m.release())
	}

	return errors.Join(errs...)
}

// guard applies the composed resilience policies for one dependency. The
// circuit breaker is distributed through Redis; timeout, retry, and bulkhead
// are process-local.
type guard struct {
	name     string
	cb       *gobreaker.DistributedCircuitBreaker[struct{}]
	sem      *semaphore.Weighted
	maxQueue int
	maxWait  time.Duration
	timeout  time.Duration
	policy   retry.Policy
	retryOn  bool
	closed   atomic.Bool
	waiting  atomic.Int64
}

// newGuard builds a guard from the template Options. The breaker name is
// namespaced by prefix so managers with different prefixes keep isolated
// breaker state.
func newGuard(name string, opts resilience.Options, prefix string, store gobreaker.SharedDataStore) (*guard, error) {
	g := &guard{
		name:    name,
		timeout: opts.Timeout,
		policy:  opts.Retry,
		retryOn: opts.Retry.MaxAttempts > 1,
	}

	if opts.Breaker.Enabled {
		breakerName := name
		if prefix != "" {
			breakerName = prefix + ":" + name
		}

		cb, err := gobreaker.NewDistributedCircuitBreaker[struct{}](store, breakerSettings(breakerName, opts.Breaker, opts.OnStateChange))
		if err != nil {
			return nil, fmt.Errorf("new distributed breaker: %w", err)
		}

		g.cb = cb
	}

	if opts.Bulkhead.MaxConcurrent > 0 {
		g.sem = semaphore.NewWeighted(int64(opts.Bulkhead.MaxConcurrent))
		g.maxQueue = opts.Bulkhead.MaxQueue
		g.maxWait = opts.Bulkhead.MaxWait
	}

	return g, nil
}

// Execute runs fn under Timeout, Retry, Breaker, and Bulkhead, in that order.
func (g *guard) Execute(ctx context.Context, fn func(context.Context) error) error {
	if g.closed.Load() {
		return resilience.ErrClosed
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	breakerRun := func(ctx context.Context) error { return g.breaker(ctx, fn) }

	var run func(context.Context) error
	if g.retryOn {
		run = func(ctx context.Context) error { return g.withRetry(ctx, breakerRun) }
	} else {
		run = breakerRun
	}

	if g.timeout <= 0 {
		return run(ctx)
	}

	parent := ctx

	ctx, cancel := context.WithTimeout(parent, g.timeout)
	defer cancel()

	err := run(ctx)
	if parent.Err() == nil && ctx.Err() != nil {
		if err != nil {
			return errors.Join(fmt.Errorf("%w after %s", resilience.ErrTimeout, g.timeout), err)
		}

		return fmt.Errorf("%w after %s", resilience.ErrTimeout, g.timeout)
	}

	return err
}

// breaker runs fn through the distributed circuit breaker when enabled.
func (g *guard) breaker(ctx context.Context, fn func(context.Context) error) error {
	if g.cb == nil {
		return g.bulkhead(ctx, fn)
	}

	_, err := g.cb.Execute(func() (struct{}, error) {
		return struct{}{}, g.bulkhead(ctx, fn)
	})

	return mapBreakerError(err)
}

// bulkhead admits fn through the weighted semaphore when enabled.
func (g *guard) bulkhead(ctx context.Context, fn func(context.Context) error) error {
	if g.sem == nil {
		return fn(ctx)
	}

	if err := g.acquire(ctx); err != nil {
		return err
	}

	defer g.sem.Release(1)

	return fn(ctx)
}

// acquire waits for a bulkhead slot, bounded by MaxQueue and MaxWait.
func (g *guard) acquire(ctx context.Context) error {
	if g.maxQueue > 0 {
		if g.waiting.Add(1) > int64(g.maxQueue) {
			g.waiting.Add(-1)

			return fmt.Errorf("%w: queue limit %d reached", resilience.ErrBulkheadFull, g.maxQueue)
		}

		defer g.waiting.Add(-1)
	}

	waitCtx := ctx

	if g.maxWait > 0 {
		var cancel context.CancelFunc

		waitCtx, cancel = context.WithTimeout(ctx, g.maxWait)
		defer cancel()
	}

	if err := g.sem.Acquire(waitCtx, 1); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("%w: max wait %s exceeded", resilience.ErrBulkheadFull, g.maxWait)
	}

	return nil
}

// withRetry retries fn on non-nil errors that are not caller cancellation,
// using the configured backoff policy.
func (g *guard) withRetry(ctx context.Context, fn func(context.Context) error) error {
	attempts := g.policy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		if attempt == attempts {
			break
		}

		if g.policy.OnRetry != nil {
			g.policy.OnRetry(attempt, err)
		}

		if err := sleepCtx(ctx, g.policy.NextDelay(attempt)); err != nil {
			return err
		}
	}

	return fmt.Errorf("resilience: retry exhausted %d attempts: %w", attempts, lastErr)
}

// sleepCtx waits d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// State reports the current circuit-breaker state. When the shared state
// cannot be read it reports StateOpen, failing closed.
func (g *guard) State() resilience.State {
	if g.cb == nil {
		return resilience.StateClosed
	}

	s, err := g.cb.State()
	if err != nil {
		return resilience.StateOpen
	}

	return stateFromBreaker(s)
}

// Name returns the guard name.
func (g *guard) Name() string { return g.name }

// Close marks the guard closed and is idempotent.
func (g *guard) Close() error {
	g.closed.Store(true)

	return nil
}
