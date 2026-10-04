package storage

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("storage: not found")

// ErrForbidden is returned when the subject lacks permission.
var ErrForbidden = errors.New("storage: forbidden")

// ErrInvalidBucket is returned for an invalid bucket name.
var ErrInvalidBucket = errors.New("storage: invalid bucket")

// ErrInvalidKey is returned for an invalid object key.
var ErrInvalidKey = errors.New("storage: invalid key")

// ErrExpired is returned for an expired presigned URL.
var ErrExpired = errors.New("storage: expired")

// ErrTooLarge is returned when an upload exceeds the size limit.
var ErrTooLarge = errors.New("storage: too large")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("storage: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("storage: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("storage: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("storage: invalid adapter")

// ErrInvalidOptions is returned for invalid storage options.
var ErrInvalidOptions = errors.New("storage: invalid options")

// ErrPresignTTLExceeded is returned when a presign TTL exceeds MaxPresignTTL.
var ErrPresignTTLExceeded = errors.New("storage: presign ttl exceeds max")

// ErrEmptyPolicyEntry is returned when a policy allow/deny list contains an empty entry.
var ErrEmptyPolicyEntry = errors.New("storage: policy contains empty entry")

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
func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter.String())
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

// InvalidOptionsError reports a storage options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error {
	return ErrInvalidOptions
}
