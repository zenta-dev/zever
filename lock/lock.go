package lock

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Locker is the interface that lock adapters must implement.
type Locker interface {
	// TryAcquire attempts to acquire key for ttl exactly once. It reports
	// ok=false (with a nil Lock and nil error) when the key is currently
	// held by someone else.
	TryAcquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error)

	// Acquire blocks, retrying, until the lock is acquired or ctx is done.
	// It returns ctx.Err() (wrapped) if the context is cancelled first.
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error)

	// Close releases the adapter's resources. It does not release locks
	// still held by callers; those expire on their own TTL.
	Close(ctx context.Context) error
}

// Lock is a single acquired lease. It is safe for concurrent use.
type Lock interface {
	// Key returns the locked key.
	Key() string

	// Extend renews the lease for a further ttl. It returns ErrNotHeld if
	// the lease has already been lost.
	Extend(ctx context.Context, ttl time.Duration) error

	// Unlock releases the lease. It returns ErrNotHeld if the lease was
	// already lost, so releasing never steals someone else's lock.
	Unlock(ctx context.Context) error
}

// Factory creates a Locker from the given Options.
type Factory func(opts Options) (Locker, error)

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

// Open creates a Locker for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Locker, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	l, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", adapter, err)
	}

	return l, nil
}
