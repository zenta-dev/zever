package password

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
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

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(a Adapter) error { return &DuplicateError{Adapter: a} },
	func(a Adapter) error { return &UnknownAdapterError{Adapter: a} },
)

// Register makes an adapter available for Open.
func Register(a Adapter, f Factory) error {
	if f == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, a)
	}

	return factories.Register(a, f)
}

// Open opens a Hasher using an already-registered adapter.
//
// Options are passed through untouched: the adapter fills zero values with
// defaults before validating.
func Open(a Adapter, opts Options) (Hasher, error) {
	factory, err := factories.Lookup(a)
	if err != nil {
		return nil, err
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
