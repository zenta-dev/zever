package casbin

import (
	"errors"
	"fmt"
)

// ErrNilEnforcer is returned when a nil enforcer is wrapped.
var ErrNilEnforcer = errors.New("casbin: nil enforcer")

// ErrDuplicatePolicy is returned when seeding a duplicate policy rule.
var ErrDuplicatePolicy = errors.New("casbin: duplicate policy")

// ErrPolicyUpdate is returned when a grouping policy removal fails or panics.
var ErrPolicyUpdate = errors.New("casbin: policy update failed")

// DuplicatePolicyError reports a duplicate seeded policy rule.
type DuplicatePolicyError struct {
	// Role is the duplicated rule role.
	Role string
	// Action is the duplicated rule action.
	Action string
}

// Error returns a human-readable duplicate-policy message.
func (e DuplicatePolicyError) Error() string {
	return fmt.Sprintf("%s: role %q action %q", ErrDuplicatePolicy, e.Role, e.Action)
}

// Unwrap returns ErrDuplicatePolicy.
func (e DuplicatePolicyError) Unwrap() error { return ErrDuplicatePolicy }
