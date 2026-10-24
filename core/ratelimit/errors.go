package ratelimit

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the limiter is closed.
var ErrClosed = errors.New("ratelimit: closed")

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("ratelimit: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("ratelimit: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("ratelimit: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("ratelimit: invalid adapter")

// ErrInvalidOptions is returned for invalid ratelimit options.
var ErrInvalidOptions = errors.New("ratelimit: invalid options")

// ErrInvalidKey is returned for an invalid rate-limit key.
var ErrInvalidKey = errors.New("ratelimit: invalid key")

// ErrInvalidCost is returned for an invalid token cost.
var ErrInvalidCost = errors.New("ratelimit: invalid cost")

// DuplicateAdapterError reports a duplicate adapter name.
type DuplicateAdapterError struct {
	// Adapter is the duplicate adapter name.
	Adapter string
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-adapter message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports an unknown adapter name.
type UnknownAdapterError struct {
	// Adapter is the unknown adapter name.
	Adapter string
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
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
