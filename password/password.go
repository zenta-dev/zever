package password

import (
	"context"
	"fmt"
	"sync"
)

// Hasher is the interface that password-hashing adapters must implement.
type Hasher interface {
	// Hash hashes the password and returns an encoded string suitable for storage.
	Hash(ctx context.Context, password string) (string, error)
	// Verify reports whether the password matches the encoded hash.
	Verify(ctx context.Context, hash, password string) (bool, error)
	// NeedsRehash reports whether the hash was created with parameters
	// different from the hasher's current configuration and should be rehashed.
	NeedsRehash(ctx context.Context, hash string) (bool, error)
}

// Factory creates a Hasher from typed options.
type Factory func(opts Options) (Hasher, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register makes an adapter available for Open.
func Register(a Adapter, f Factory) error {
	if f == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, a)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[a]; dup {
		return &DuplicateError{Adapter: a}
	}

	factories[a] = f

	return nil
}

// Open opens a Hasher using an already-registered adapter.
//
// Options are passed through untouched: the adapter fills zero values with
// defaults before validating.
func Open(a Adapter, opts Options) (Hasher, error) {
	mu.RLock()
	factory, ok := factories[a]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: a}
	}

	h, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("password: open %s: %w", a, err)
	}

	return h, nil
}

// Hash is a convenience that hashes with the default argon2id adapter.
// The caller must have registered the adapter first.
func Hash(ctx context.Context, password string) (string, error) {
	h, err := Open(AdapterArgon2ID, Options{})
	if err != nil {
		return "", err
	}

	return h.Hash(ctx, password)
}

// Verify is a convenience that verifies against the default argon2id adapter.
// The caller must have registered the adapter first.
func Verify(ctx context.Context, hash, password string) (bool, error) {
	h, err := Open(AdapterArgon2ID, Options{})
	if err != nil {
		return false, err
	}

	return h.Verify(ctx, hash, password)
}
