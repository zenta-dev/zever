package tenant

import (
	"context"
	"fmt"
	"sync"
)

// Tenant defines the multi-tenant resolution contract for tenant backends.
// Implementations return the zero value on error.
type Tenant interface {
	// Resolve returns the tenant ID for ctx and meta. It returns "" on error.
	Resolve(ctx context.Context, meta map[string]string) (string, error)
	// Scoped returns a context carrying tenantID. It returns nil on error.
	Scoped(ctx context.Context, tenantID string) (context.Context, error)
	// Close releases backend resources.
	Close() error
}

// Factory creates a Tenant from the given Options.
type Factory func(opts Options) (Tenant, error)

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
		return &DuplicateAdapterError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a Tenant for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Tenant, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	t, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("tenant: open %s: %w", adapter, err)
	}

	return t, nil
}
