package password

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidHash is returned when a stored hash is malformed or unsupported.
	ErrInvalidHash = errors.New("password: invalid hash")
	// ErrPasswordTooLong is returned when a password exceeds the maximum allowed length.
	ErrPasswordTooLong = errors.New("password: password too long")
	// ErrNilFactory is returned when an adapter factory is nil.
	ErrNilFactory = errors.New("password: nil factory")
	// ErrDuplicate is returned on duplicate adapter registration.
	ErrDuplicate = errors.New("password: duplicate registration")
	// ErrUnknownAdapter is returned for an unregistered adapter.
	ErrUnknownAdapter = errors.New("password: unknown adapter")
	// ErrInvalidAdapter is returned for an invalid adapter name.
	ErrInvalidAdapter = errors.New("password: invalid adapter")
)

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

// DuplicateError reports a duplicate adapter registration.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter)
}

// Unwrap returns ErrDuplicate.
func (e DuplicateError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }
