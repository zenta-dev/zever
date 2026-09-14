package ratelimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// Decision is the outcome of an Allow call.
type Decision struct {
	// Allowed reports whether the request is within the rate limit.
	Allowed bool
	// RetryAfter is how long to wait before retrying after a denial.
	RetryAfter time.Duration
	// Remaining is the tokens remaining in the bucket.
	Remaining float64
}

// Limiter enforces token-bucket rate limits.
type Limiter interface {
	// Allow consumes tokens for key and reports the decision.
	// An error is returned for infrastructure failures only; the
	// fail-open decision is left to the caller. A denial is returned
	// as (Decision{Allowed: false, RetryAfter: ..., Remaining: ...}, nil).
	Allow(ctx context.Context, key string, tokens float64) (Decision, error)

	// Reset clears the bucket state for key.
	Reset(ctx context.Context, key string) error

	// Close shuts down the limiter and releases associated resources.
	Close() error

	// Name returns the adapter name for the limiter.
	Name() string
}

// Factory creates a Limiter from the given Options.
type Factory func(opts Options) (Limiter, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a Limiter for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Limiter, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	l, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: open %s: %w", adapter, err)
	}

	return l, nil
}

// ValidateCost checks a token cost and burst capacity for Allow calls.
// Both must be finite and > 0. A cost above burst is not an error:
// adapters cap the charge at burst. Shared by adapters so both enforce
// the same cost validation.
func ValidateCost(tokens, burst float64) error {
	if math.IsNaN(tokens) || math.IsInf(tokens, 0) || tokens <= 0 {
		return fmt.Errorf("%w: tokens must be > 0 and finite", ErrInvalidCost)
	}

	if math.IsNaN(burst) || math.IsInf(burst, 0) || burst <= 0 {
		return fmt.Errorf("%w: burst must be > 0 and finite", ErrInvalidCost)
	}

	return nil
}
