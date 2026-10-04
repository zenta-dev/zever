package cache

import (
	"fmt"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/shared/registry"
)

// SharedFactory builds a Cache over an already-open pool instead of opening
// its own connection. The caller retains pool ownership: Close on the built
// Cache never closes conn. Adapters with a DB-backed backend register their
// OpenFromDB constructors here so the container can borrow a shared pool
// without importing the adapter. Core importing core/db mirrors the
// core/scheduler to core/job precedent: a registry-level dependency, never
// a new interface method.
type SharedFactory func(conn coredb.DB, opts Options) (Cache, error)

var sharedFactories = registry.New[Adapter, SharedFactory](
	ErrNilFactory,
	func(adapter Adapter) error { return DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return UnknownAdapterError{Adapter: adapter} },
)

// RegisterShared associates an Adapter with a SharedFactory for later use
// by OpenShared.
func RegisterShared(adapter Adapter, factory SharedFactory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return sharedFactories.Register(adapter, factory)
}

// OpenShared builds the Adapter over conn using the registered
// SharedFactory and opts.
func OpenShared(adapter Adapter, conn coredb.DB, opts Options) (Cache, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := sharedFactories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	c, err := factory(conn, opts)
	if err != nil {
		return nil, fmt.Errorf("cache: open shared %s: %w", adapter, err)
	}

	return c, nil
}
