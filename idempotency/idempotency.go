package idempotency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BeginOptions configures a single Begin reservation.
type BeginOptions struct {
	// Fingerprint scopes the key to a specific request payload.
	// Zero value (nil/empty) means no fingerprint; matching against a
	// stored record follows FingerprintMatches semantics.
	Fingerprint []byte
	// TTL is the reservation lifetime. Zero means the store default.
	TTL time.Duration
}

// Outcome is the result of a Begin call.
// Replay=true means Result is the stored replay and the caller must
// NOT re-execute; Replay=false means the caller holds the reservation
// and must execute, then call Complete.
type Outcome struct {
	// Replay reports whether Result is a stored replay.
	Replay bool
	// Result holds the stored result on replay; nil on reservation miss.
	Result []byte
}

// Store is the idempotency backend.
//
// Begin atomically checks for an existing record and reserves the key on
// miss: miss returns ({false, nil}, nil) and the caller executes, then
// calls Complete. A hit on a completed record with matching fingerprint
// returns ({true, copy}, nil). A hit on a pending record returns
// ErrInProgress. Fingerprint is checked FIRST on any existing record:
// a mismatch returns ErrKeyMismatch even mid-flight. Complete stores the
// result and marks the record done. Forget removes the record and is
// idempotent (nil on missing keys). All errors are fail-closed.
type Store interface {
	// Begin checks for an existing record and reserves key on miss.
	Begin(ctx context.Context, key string, opts BeginOptions) (Outcome, error)
	// Complete stores result for key and marks the record done.
	Complete(ctx context.Context, key string, fingerprint, result []byte) error
	// Forget removes key; missing keys return nil.
	Forget(ctx context.Context, key string) error
	// Close shuts down the store and releases associated resources.
	Close() error
}

// Factory creates a Store from the given Options.
type Factory func(opts Options) (Store, error)

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

// Open creates a Store for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Store, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("idempotency: open %s: %w", adapter, err)
	}

	return s, nil
}
