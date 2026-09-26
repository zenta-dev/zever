package tenant

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
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

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateAdapterError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Tenant for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Tenant, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	t, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("tenant: open %s: %w", adapter, err)
	}

	return t, nil
}
