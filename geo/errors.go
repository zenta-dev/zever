package geo

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("geo: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("geo: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("geo: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("geo: invalid adapter")

// ErrInvalidOptions is returned for invalid geo options.
var ErrInvalidOptions = errors.New("geo: invalid options")

// ErrNotFound is returned when a geo result cannot be found.
var ErrNotFound = errors.New("geo: not found")

// ErrInvalidCoordinate is returned for out-of-range or non-finite coordinates.
var ErrInvalidCoordinate = errors.New("geo: invalid coordinate")

// ErrTooLarge is returned when a geo response exceeds the size limit.
var ErrTooLarge = errors.New("geo: response too large")

// InvalidAdapterError reports a failed adapter name parse.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e *InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e *InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }

// DuplicateAdapterError reports a double registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e *DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e *DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports an open of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e *UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e *UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidOptionsError reports a single options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e *InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e *InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }

// NotFoundError reports a missing geo result.
type NotFoundError struct {
	// Query is the query that produced no results.
	Query string
}

// Error returns a human-readable not-found message.
func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s: %q", ErrNotFound, e.Query)
}

// Unwrap returns ErrNotFound.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }
