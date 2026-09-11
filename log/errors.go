package log

import (
	"errors"
	"fmt"
)

// Sentinel errors for log operations.
var (
	ErrNilFactory     = errors.New("log: nil factory")
	ErrDuplicate      = errors.New("log: duplicate registration")
	ErrUnknownAdapter = errors.New("log: unknown adapter")
	ErrInvalidLevel   = errors.New("log: invalid level")
	ErrInvalidAdapter = errors.New("log: invalid adapter")
)

// DuplicateError reports a repeated Register for the same adapter.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable description of the duplicate registration.
func (e *DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate.Error(), e.Adapter.String())
}

// Unwrap returns ErrDuplicate for errors.Is matching.
func (e *DuplicateError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports an Open for an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the requested but unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable description of the unknown adapter.
func (e *UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter.Error(), e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter for errors.Is matching.
func (e *UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidLevelError reports a ParseLevel failure, carrying the bad input.
type InvalidLevelError struct {
	// Level is the unrecognized level name.
	Level string
}

// Error returns a human-readable description of the invalid level.
func (e *InvalidLevelError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidLevel.Error(), e.Level)
}

// Unwrap returns ErrInvalidLevel for errors.Is matching.
func (e *InvalidLevelError) Unwrap() error { return ErrInvalidLevel }

// InvalidAdapterError reports a ParseAdapter failure, carrying the bad input.
type InvalidAdapterError struct {
	// Adapter is the unrecognized adapter name.
	Adapter string
}

// Error returns a human-readable description of the invalid adapter.
func (e *InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter.Error(), e.Adapter)
}

// Unwrap returns ErrInvalidAdapter for errors.Is matching.
func (e *InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }
