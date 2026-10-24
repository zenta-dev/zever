package observability

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("observability: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("observability: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("observability: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("observability: invalid adapter")

// ErrInvalidOptions is returned for invalid observability options.
var ErrInvalidOptions = errors.New("observability: invalid options")

// ErrTooManyInstruments is returned when instrument limit exceeded.
var ErrTooManyInstruments = errors.New("observability: too many instruments")

// ErrInstrumentConflict is returned when an instrument name is reused for a different kind.
var ErrInstrumentConflict = errors.New("observability: instrument conflict")

// DuplicateAdapterError reports a duplicate adapter name.
type DuplicateAdapterError struct {
	// Adapter is the duplicate adapter name.
	Adapter string
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable description of the duplicate adapter.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter for errors.Is matching.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports an unknown adapter name.
type UnknownAdapterError struct {
	// Adapter is the unknown adapter name.
	Adapter string
}

// Error returns a human-readable description of the unknown adapter.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter for errors.Is matching.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidAdapterError reports an invalid adapter name, carrying the bad input.
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
