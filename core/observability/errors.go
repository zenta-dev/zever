package observability

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("observability: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("observability: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("observability: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("observability: invalid adapter")

// ErrInvalidOptions is returned for invalid observability options.
var ErrInvalidOptions = errors.New("observability: invalid options")

// ErrTooManyInstruments is returned when instrument limit exceeded.
var ErrTooManyInstruments = errors.New("observability: too many instruments")

// ErrInstrumentConflict is returned when an instrument name is reused for a different kind.
var ErrInstrumentConflict = errors.New("observability: instrument conflict")

// DuplicateAdapterError reports a repeated Register for the same adapter.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable description of the duplicate registration.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate for errors.Is matching.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports an Open for an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the requested but unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable description of the unknown adapter.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
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
