package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/shared/registry"
)

// Cache defines the backend contract for byte-slice cache entries with TTL and atomic counters.
type Cache interface {
	// Get retrieves the value stored under key.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set stores value under key, overwriting any existing entry.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// SetIfAbsent stores value under key only when no live entry exists.
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	// Delete removes the entry stored under key.
	Delete(ctx context.Context, key string) error
	// Increment atomically increments the integer value stored under key.
	Increment(ctx context.Context, key string) error
	// Decrement atomically decrements the integer value stored under key.
	Decrement(ctx context.Context, key string) error
	// Exists reports whether a live entry exists under key.
	Exists(ctx context.Context, key string) (bool, error)
	// Close releases backend resources and invalidates the cache.
	Close(ctx context.Context) error
}

// Factory creates a Cache from the given Options.
type Factory func(opts Options) (Cache, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Cache for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Cache, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("cache: open %s: %w", adapter, err)
	}
	return c, nil
}
