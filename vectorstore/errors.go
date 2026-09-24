package vectorstore

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("vectorstore: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("vectorstore: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("vectorstore: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("vectorstore: invalid adapter")

// ErrInvalidOptions is returned for invalid vectorstore options.
var ErrInvalidOptions = errors.New("vectorstore: invalid options")

// ErrNotFound is returned when a vector cannot be found.
var ErrNotFound = errors.New("vectorstore: not found")

// ErrEmptyEmbedding is returned when an embedding is empty.
var ErrEmptyEmbedding = errors.New("vectorstore: empty embedding")

// ErrDimensionMismatch is returned when an embedding has the wrong dimension.
var ErrDimensionMismatch = errors.New("vectorstore: dimension mismatch")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

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
func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// InvalidOptionsError reports a vectorstore options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error {
	return ErrInvalidOptions
}

// NotFoundError reports a missing vector.
type NotFoundError struct {
	// ID is the identifier of the missing vector.
	ID string
}

// Error returns a human-readable not-found message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s: %q", ErrNotFound, e.ID)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}

// DimensionMismatchError reports an embedding with the wrong dimension.
type DimensionMismatchError struct {
	// Got is the received embedding dimension.
	Got int
	// Want is the expected embedding dimension.
	Want int
}

// Error returns a human-readable dimension-mismatch message.
func (e DimensionMismatchError) Error() string {
	return fmt.Sprintf("%s: got %d, want %d", ErrDimensionMismatch, e.Got, e.Want)
}

// Unwrap returns ErrDimensionMismatch.
func (e DimensionMismatchError) Unwrap() error {
	return ErrDimensionMismatch
}
