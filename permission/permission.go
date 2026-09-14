package permission

import (
	"context"
	"fmt"
	"sync"
)

// Subject identifies the actor requesting access.
type Subject struct {
	// ID identifies the actor.
	ID string
	// Roles lists the actor's roles.
	Roles []string
	// Attributes carries extra actor attributes.
	Attributes map[string]string
}

// Resource identifies the object being accessed.
type Resource struct {
	// Type is the resource type.
	Type string
	// ID is the resource identifier.
	ID string
	// Attributes carries extra resource attributes.
	Attributes map[string]string
}

// Decision is the outcome of an authorization check.
type Decision struct {
	// Allowed reports whether access is granted.
	Allowed bool
	// Reason is one of "allow", "implicit_deny", "explicit_deny".
	Reason string
}

// Checker authorizes actions on resources.
// An error signals infrastructure failure only; a denial is
// Decision{Allowed:false} with a nil error. Callers deny on err != nil
// (fail-closed).
type Checker interface {
	// Can reports whether subject may perform action on resource.
	Can(ctx context.Context, subject Subject, action string, resource Resource) (Decision, error)
}

// Factory creates a Checker from the given options.
type Factory func(opts Options) (Checker, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an adapter with its factory for later use by Open.
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

// Open creates a Checker for a registered adapter using the given options.
func Open(adapter Adapter, opts Options) (Checker, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	checker, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("permission: open %s: %w", adapter, err)
	}

	return checker, nil
}
