package resilience

import (
	"context"
)

// State is the circuit-breaker state exposed by a Guard.
type State string

const (
	// StateClosed lets calls through and counts their outcomes.
	StateClosed State = "closed"
	// StateOpen rejects calls until the open timeout elapses.
	StateOpen State = "open"
	// StateHalfOpen admits a limited number of probe calls.
	StateHalfOpen State = "half-open"
)

// Guard executes operations under the configured resilience policies.
// Implementations must be safe for concurrent use.
type Guard interface {
	// Execute runs fn under the composed policies. A nil error means the
	// operation succeeded and was admitted through every policy.
	Execute(ctx context.Context, fn func(context.Context) error) error
	// State reports the current circuit-breaker state.
	State() State
	// Name returns the guard name.
	Name() string
	// Close releases resources and is idempotent.
	Close() error
}

// Manager creates and owns named per-dependency guards.
// Implementations must be safe for concurrent use.
type Manager interface {
	// Guard returns the named guard, lazily creating it from the template
	// Options on first use. The same name always returns the same Guard.
	Guard(name string) (Guard, error)
	// Close closes every guard and is idempotent.
	Close() error
}

// Factory creates a Manager from the given Options.
type Factory func(opts Options) (Manager, error)
