package storage

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("storage: not found")

// ErrForbidden is returned when the subject lacks permission.
var ErrForbidden = errors.New("storage: forbidden")

// ErrInvalidBucket is returned for an invalid bucket name.
var ErrInvalidBucket = errors.New("storage: invalid bucket")

// ErrInvalidKey is returned for an invalid object key.
var ErrInvalidKey = errors.New("storage: invalid key")

// ErrExpired is returned for an expired presigned URL.
var ErrExpired = errors.New("storage: expired")

// ErrTooLarge is returned when an upload exceeds the size limit.
var ErrTooLarge = errors.New("storage: too large")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("storage: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("storage: duplicate registration")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("storage: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("storage: invalid adapter")

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("storage: invalid adapter: %q", e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// DuplicateError reports a duplicate adapter registration.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateError) Unwrap() error {
	return ErrDuplicate
}

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
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}
