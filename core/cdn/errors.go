package cdn

import (
	"errors"
	"fmt"
)

var (
	// ErrNilFactory is returned when registering a nil factory.
	ErrNilFactory = errors.New("cdn: nil factory")
	// ErrDuplicateAdapter is returned when registering an adapter twice.
	ErrDuplicateAdapter = errors.New("cdn: duplicate adapter")
	// ErrUnknownAdapter is returned when opening an unregistered adapter.
	ErrUnknownAdapter = errors.New("cdn: unknown adapter (forgotten import?)")
	// ErrInvalidAdapter is returned when parsing an empty adapter name.
	ErrInvalidAdapter = errors.New("cdn: invalid adapter")
	// ErrClosed is returned when operating on a closed CDN client.
	ErrClosed = errors.New("cdn: closed")
)

// InvalidAdapterError reports an empty or malformed adapter name.
type InvalidAdapterError struct {
	// Adapter is the rejected adapter name.
	Adapter string
}

// Error returns the human-readable error message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("cdn: invalid adapter %q", e.Adapter)
}

// Unwrap returns ErrInvalidAdapter for errors.Is branching.
func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// DuplicateError reports a repeated adapter registration.
type DuplicateError struct {
	// Adapter is the duplicated adapter.
	Adapter Adapter
}

// Error returns the human-readable error message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("cdn: duplicate adapter %q", e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter for errors.Is branching.
func (e DuplicateError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports an unregistered adapter lookup.
type UnknownAdapterError struct {
	// Adapter is the unknown adapter.
	Adapter Adapter
}

// Error returns the human-readable error message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("cdn: unknown adapter %q", e.Adapter)
}

// Unwrap returns ErrUnknownAdapter for errors.Is branching.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}
