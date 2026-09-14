package ratelimit

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the limiter is closed.
var ErrClosed = errors.New("ratelimit: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("ratelimit: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("ratelimit: duplicate registration")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("ratelimit: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("ratelimit: invalid adapter")

// ErrInvalidOptions is returned for invalid ratelimit options.
var ErrInvalidOptions = errors.New("ratelimit: invalid options")

// ErrInvalidKey is returned for an invalid rate-limit key.
var ErrInvalidKey = errors.New("ratelimit: invalid key")

// ErrInvalidCost is returned for an invalid token cost.
var ErrInvalidCost = errors.New("ratelimit: invalid cost")

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
func (e DuplicateError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter.String())
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

// InvalidOptionsError reports a ratelimit options validation failure.
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

// InvalidKeyError reports a rate-limit key validation failure.
// It carries only the key length, never the key itself, since keys may
// be PII such as IP addresses.
type InvalidKeyError struct {
	// KeyLen is the length of the invalid key in bytes.
	KeyLen int
}

// Error returns a human-readable invalid-key message without echoing the key.
func (e InvalidKeyError) Error() string {
	return fmt.Sprintf("%s: invalid length %d", ErrInvalidKey, e.KeyLen)
}

// Unwrap returns ErrInvalidKey.
func (e InvalidKeyError) Unwrap() error { return ErrInvalidKey }
