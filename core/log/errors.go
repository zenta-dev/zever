package log

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("log: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("log: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("log: unknown adapter")

// ErrInvalidLevel is returned for an invalid level name.
var ErrInvalidLevel = errors.New("log: invalid level")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("log: invalid adapter")

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

// InvalidLevelError reports a ParseLevel failure, carrying the bad input.
type InvalidLevelError struct {
	// Level is the unrecognized level name.
	Level string
}

// Error returns a human-readable description of the invalid level.
func (e InvalidLevelError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidLevel, e.Level)
}

// Unwrap returns ErrInvalidLevel for errors.Is matching.
func (e InvalidLevelError) Unwrap() error { return ErrInvalidLevel }

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
