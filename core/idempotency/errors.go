package idempotency

import (
	"errors"
	"fmt"
)

// ErrInProgress is returned when a key is already reserved by an in-flight execution.
var ErrInProgress = errors.New("idempotency: in progress")

// ErrKeyMismatch is returned when the incoming fingerprint does not match the stored record.
var ErrKeyMismatch = errors.New("idempotency: key mismatch")

// ErrClosed is returned when the store is closed.
var ErrClosed = errors.New("idempotency: closed")

// ErrInvalidKey is returned for an invalid idempotency key.
var ErrInvalidKey = errors.New("idempotency: invalid key")

// ErrFingerprintTooLarge is returned when a per-call fingerprint exceeds the size cap.
var ErrFingerprintTooLarge = errors.New("idempotency: fingerprint too large")

// ErrCorruptRecord is returned when a stored wire record cannot be decoded.
var ErrCorruptRecord = errors.New("idempotency: corrupt record")

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("idempotency: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("idempotency: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("idempotency: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("idempotency: invalid adapter")

// ErrInvalidOptions is returned for invalid idempotency options.
var ErrInvalidOptions = errors.New("idempotency: invalid options")

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

// InvalidOptionsError reports an idempotency options validation failure.
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

// InvalidKeyError reports an invalid idempotency key.
// Key bytes are never echoed: Error prints the length only, since keys
// may carry sensitive material.
type InvalidKeyError struct {
	// KeyLen is the byte length of the rejected key.
	KeyLen int
}

// Error returns a human-readable invalid-key message with the key length only.
func (e InvalidKeyError) Error() string {
	return fmt.Sprintf("%s: invalid key length %d", ErrInvalidKey, e.KeyLen)
}

// Unwrap returns ErrInvalidKey.
func (e InvalidKeyError) Unwrap() error { return ErrInvalidKey }
