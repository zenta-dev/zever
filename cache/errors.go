package cache

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a cache key does not exist.
var ErrNotFound = errors.New("cache: not found")

// ErrClosed is returned when the cache is closed.
var ErrClosed = errors.New("cache: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("cache: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("cache: duplicate registration")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("cache: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("cache: invalid adapter")

// ErrInvalidValue is returned for an invalid cache value.
var ErrInvalidValue = errors.New("cache: invalid value")

// NotFoundError reports a missing cache key.
type NotFoundError struct {
	// Key is the missing cache key.
	Key string
}

// Error returns a human-readable missing-key message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("cache: not found: key %q", e.Key)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}

// InvalidValueError reports an invalid value for a cache key.
type InvalidValueError struct {
	// Key is the cache key holding the invalid value.
	Key string
	// Err is the underlying decoding or conversion cause.
	Err error
}

// Error returns a human-readable invalid-value message.
func (e InvalidValueError) Error() string {
	return fmt.Sprintf("cache: invalid value: key %q: %v", e.Key, e.Err)
}

// Unwrap returns ErrInvalidValue joined with the cause.
func (e InvalidValueError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrInvalidValue, e.Err)
	}
	return ErrInvalidValue
}

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("cache: invalid adapter: %q", e.Adapter)
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
