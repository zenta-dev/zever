package auth

import (
	"errors"
	"fmt"
)

// Token problems use these bare sentinels only: token bytes are secrets
// and must never be echoed in errors or logs.
var (
	// ErrTokenExpired is returned when a token is past its expiry.
	ErrTokenExpired = errors.New("auth: token expired")
	// ErrTokenRevoked is returned when a token was revoked.
	ErrTokenRevoked = errors.New("auth: token revoked")
	// ErrInvalidToken is returned for a malformed, unknown, or otherwise
	// invalid token. The token value is never echoed.
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrNotSupported is returned when an adapter does not support an operation.
	ErrNotSupported = errors.New("auth: not supported")
	// ErrNilFactory is returned when a nil backend factory is provided.
	ErrNilFactory = errors.New("auth: nil factory")
	// ErrDuplicateAdapter is returned for a duplicate adapter name.
	ErrDuplicateAdapter = errors.New("auth: duplicate adapter")
	// ErrUnknownAdapter is returned for an unknown adapter name.
	ErrUnknownAdapter = errors.New("auth: unknown adapter")
	// ErrInvalidAdapter is returned for an invalid adapter name.
	ErrInvalidAdapter = errors.New("auth: invalid adapter")
	// ErrInvalidOptions is returned for invalid auth options.
	ErrInvalidOptions = errors.New("auth: invalid options")
)

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

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

// InvalidOptionsError reports an auth options validation failure.
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
