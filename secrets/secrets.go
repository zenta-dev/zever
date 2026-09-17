package secrets

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Secrets is the interface that secret-management adapters must implement.
type Secrets interface {
	// Get returns the secret value for name.
	Get(ctx context.Context, name string) ([]byte, error)

	// Set stores value under name.
	Set(ctx context.Context, name string, value []byte) error

	// Delete removes the secret stored under name.
	Delete(ctx context.Context, name string) error

	// List returns the names of all stored secrets.
	List(ctx context.Context) ([]string, error)

	// Close releases backend resources.
	Close(ctx context.Context) error
}

// Factory creates a Secrets from the given Options.
type Factory func(opts Options) (Secrets, error)

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

// Open creates a Secrets for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Secrets, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("secrets: open %s: %w", adapter, err)
	}

	return s, nil
}

var validSecretName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateName rejects secret names that could cause path traversal or
// injection across backends. Names must be non-empty and contain only
// alphanumeric characters, dots, hyphens, and underscores.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: secret name must not be empty", ErrInvalidKey)
	}

	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return fmt.Errorf("%w: secret name must not contain / or dot-dot", ErrInvalidKey)
	}

	if !validSecretName.MatchString(name) {
		return fmt.Errorf("%w: secret name must match [a-zA-Z0-9._-]+, got %q", ErrInvalidKey, name)
	}

	return nil
}
