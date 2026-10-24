package geo

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("geo: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("geo: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
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

// DuplicateAdapterError reports a duplicate adapter name.
type DuplicateAdapterError struct {
	// Adapter is the duplicate adapter name.
	Adapter string
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-adapter message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports an unknown adapter name.
type UnknownAdapterError struct {
	// Adapter is the unknown adapter name.
	Adapter string
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidOptionsError reports a single options validation failure.
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

// NotFoundError reports a missing geo result.
type NotFoundError struct {
	// Query is the query that produced no results.
	Query string
}

// Error returns a human-readable not-found message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s: %q", ErrNotFound, e.Query)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error { return ErrNotFound }
