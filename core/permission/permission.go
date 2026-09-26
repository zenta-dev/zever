package permission

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
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

// Factory creates a Checker from the given Options.
type Factory func(opts Options) (Checker, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Checker for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Checker, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	checker, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("permission: open %s: %w", adapter, err)
	}

	return checker, nil
}
