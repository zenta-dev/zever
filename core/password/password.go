package password

import (
	"context"
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

// Factory creates a Hasher from the given Options.
type Factory func(opts Options) (Hasher, error)
