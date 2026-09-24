package session

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a session does not exist or has expired.
// Expired sessions are indistinguishable from missing ones.
var ErrNotFound = errors.New("session: not found")

// ErrClosed is returned when a store is used after Close.
var ErrClosed = errors.New("session: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("session: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("session: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("session: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("session: invalid adapter")

// ErrInvalidOptions is returned for invalid session options.
var ErrInvalidOptions = errors.New("session: invalid options")

// ErrInvalidID is returned for an invalid session ID.
var ErrInvalidID = errors.New("session: invalid id")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicate }

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

// InvalidOptionsError reports a session options validation failure.
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

// InvalidIDError reports an invalid session ID.
// IDs are secrets and are never echoed: Error prints the length only,
// while IDLen is available via errors.As for programmatic use.
type InvalidIDError struct {
	// IDLen is the byte length of the rejected ID.
	IDLen int
}

// Error returns a human-readable invalid-id message with the ID length only.
func (e InvalidIDError) Error() string {
	return fmt.Sprintf("%s: invalid id length %d", ErrInvalidID, e.IDLen)
}

// Unwrap returns ErrInvalidID.
func (e InvalidIDError) Unwrap() error { return ErrInvalidID }
