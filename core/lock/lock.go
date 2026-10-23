package lock

import (
	"context"
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
