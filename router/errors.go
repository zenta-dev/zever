package router

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("router: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("router: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("router: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("router: invalid adapter")

// ErrInvalidOptions is returned for invalid router options.
var ErrInvalidOptions = errors.New("router: invalid options")

// ErrInvalidMethod is returned for an invalid HTTP method.
var ErrInvalidMethod = errors.New("router: invalid method")

// ErrMalformedPattern is returned for a malformed route pattern.
var ErrMalformedPattern = errors.New("router: malformed pattern")

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

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

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

// InvalidOptionsError reports a router options validation failure.
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

// InvalidMethodError reports an invalid HTTP method.
type InvalidMethodError struct {
	// Method is the invalid HTTP method.
	Method string
}

// Error returns a human-readable invalid-method message.
func (e InvalidMethodError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidMethod, e.Method)
}

// Unwrap returns ErrInvalidMethod.
func (e InvalidMethodError) Unwrap() error { return ErrInvalidMethod }

// MalformedPatternError reports a malformed route pattern.
type MalformedPatternError struct {
	// Pattern is the malformed route pattern.
	Pattern string
	// Reason describes why the pattern is malformed.
	Reason string
}

// Error returns a human-readable malformed-pattern message.
func (e MalformedPatternError) Error() string {
	return fmt.Sprintf("%s: %q: %s", ErrMalformedPattern, e.Pattern, e.Reason)
}

// Unwrap returns ErrMalformedPattern.
func (e MalformedPatternError) Unwrap() error { return ErrMalformedPattern }
