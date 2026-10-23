package permission

import (
	"context"
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
