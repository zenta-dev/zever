package permission

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("permission: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("permission: duplicate registration")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("permission: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("permission: invalid adapter")

// ErrInvalidOptions is returned for invalid permission options.
var ErrInvalidOptions = errors.New("permission: invalid options")

// DuplicateError reports a repeated Register for the same adapter.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable description of the duplicate registration.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate for errors.Is matching.
func (e DuplicateError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports an Open for an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the requested but unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable description of the unknown adapter.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter for errors.Is matching.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidAdapterError reports a ParseAdapter failure, carrying the bad input.
type InvalidAdapterError struct {
	// Adapter is the unrecognized adapter name.
	Adapter string
}

// Error returns a human-readable description of the invalid adapter.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter for errors.Is matching.
func (e InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }

// InvalidOptionsError reports an Options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable description of the invalid options.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions for errors.Is matching.
func (e InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }
