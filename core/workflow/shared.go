package workflow

import (
	"fmt"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/shared/registry"
)

// SharedFactory builds a Workflow over an already-open pool instead of
// opening its own connection. The caller retains pool ownership: Close on
// the built Workflow never closes conn. Adapters with a DB-backed backend
// register their OpenFromDB constructors here so the container can borrow a
// shared pool without importing the adapter. Core importing core/db mirrors
// the core/scheduler to core/job precedent: a registry-level dependency,
// never a new interface method.
type SharedFactory func(conn coredb.DB, opts Options) (Workflow, error)

var sharedFactories = registry.New[Adapter, SharedFactory](
	ErrNilFactory,
	func(a Adapter) error { return DuplicateError{Adapter: a} },
	func(a Adapter) error { return UnknownAdapterError{Adapter: a} },
)

// RegisterShared associates an Adapter with a SharedFactory for later use
// by OpenShared.
func RegisterShared(adapter Adapter, factory SharedFactory) error {
	if factory == nil {
		return fmt.Errorf("workflow: %w for adapter %s", ErrNilFactory, adapter)
	}

	return sharedFactories.Register(adapter, factory)
}

// OpenShared builds the Adapter over conn using the registered
// SharedFactory and opts.
func OpenShared(adapter Adapter, conn coredb.DB, opts Options) (Workflow, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := sharedFactories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	w, err := factory(conn, opts)
	if err != nil {
		return nil, fmt.Errorf("workflow: open shared %s: %w", adapter, err)
	}

	return w, nil
}
