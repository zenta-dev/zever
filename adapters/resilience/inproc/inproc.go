package inproc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/sync/semaphore"

	"github.com/zenta-dev/zever/core/resilience"
	"github.com/zenta-dev/zever/shared/retry"
)

var (
	_ resilience.Manager = (*manager)(nil)
	_ resilience.Guard   = (*guard)(nil)
)

// manager owns the lazily created per-dependency guards for one adapter instance.
type manager struct {
	mu     sync.Mutex
	guards map[string]*guard
	opts   resilience.Options
	closed bool
}

// New creates an in-process resilience Manager from opts.
func New(opts resilience.Options) (resilience.Manager, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("inproc: %w", err)
	}

	return &manager{guards: make(map[string]*guard), opts: opts}, nil
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

	g := newGuard(name, m.opts)

	m.guards[name] = g

	return g, nil
}

// Close closes every guard and is idempotent.
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

	return errors.Join(errs...)
}

// guard applies the composed resilience policies for one dependency.
type guard struct {
	name     string
	cb       *gobreaker.CircuitBreaker[struct{}]
	sem      *semaphore.Weighted
	maxQueue int
	maxWait  time.Duration
	timeout  time.Duration
	policy   retry.Policy
	retryOn  bool
	closed   atomic.Bool
	waiting  atomic.Int64
}

// newGuard builds a guard from the template Options.
func newGuard(name string, opts resilience.Options) *guard {
	g := &guard{
		name:    name,
		timeout: opts.Timeout,
		policy:  opts.Retry,
		retryOn: opts.Retry.MaxAttempts > 1,
	}

	if opts.Breaker.Enabled {
		g.cb = gobreaker.NewCircuitBreaker[struct{}](breakerSettings(name, opts.Breaker, opts.OnStateChange))
	}

	if opts.Bulkhead.MaxConcurrent > 0 {
		g.sem = semaphore.NewWeighted(int64(opts.Bulkhead.MaxConcurrent))
		g.maxQueue = opts.Bulkhead.MaxQueue
		g.maxWait = opts.Bulkhead.MaxWait
	}

	return g
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

// breaker runs fn through the circuit breaker when enabled.
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

// State reports the current circuit-breaker state.
func (g *guard) State() resilience.State {
	if g.cb == nil {
		return resilience.StateClosed
	}

	return stateFromBreaker(g.cb.State())
}

// Name returns the guard name.
func (g *guard) Name() string { return g.name }

// Close marks the guard closed and is idempotent.
func (g *guard) Close() error {
	g.closed.Store(true)

	return nil
}
