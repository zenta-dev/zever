package resilience

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("resilience: nil factory")

// ErrOpenState is returned when the circuit breaker is open and rejects a call.
var ErrOpenState = errors.New("resilience: circuit open")

// ErrTooManyRequests is returned when a half-open breaker is at its probe limit.
var ErrTooManyRequests = errors.New("resilience: too many requests")

// ErrBulkheadFull is returned when the bulkhead cannot admit a call within its wait budget.
var ErrBulkheadFull = errors.New("resilience: bulkhead full")

// ErrTimeout is returned when the overall operation exceeds its timeout budget.
var ErrTimeout = errors.New("resilience: timeout")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("resilience: unknown adapter")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("resilience: duplicate adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("resilience: invalid adapter")

// ErrInvalidOptions is returned for invalid resilience options.
var ErrInvalidOptions = errors.New("resilience: invalid options")

// ErrClosed is returned when a closed Manager or Guard is used.
var ErrClosed = errors.New("resilience: closed")

// DuplicateError reports a duplicate adapter registration.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter.String())
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }

// InvalidOptionsError reports a resilience options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }
