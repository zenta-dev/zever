package analytics

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("analytics: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("analytics: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("analytics: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("analytics: invalid adapter")

// ErrInvalidOptions is returned for invalid analytics options.
var ErrInvalidOptions = errors.New("analytics: invalid options")

// ErrMissingIdentity is returned when an operation requires an identity but none is present.
var ErrMissingIdentity = errors.New("analytics: missing identity")

// ErrMissingGroupID is returned when a group operation is missing the group ID.
var ErrMissingGroupID = errors.New("analytics: missing group id")

// ErrTooManyProperties is returned when properties exceed the count limit.
var ErrTooManyProperties = errors.New("analytics: too many properties")

// ErrPropertiesTooLarge is returned when properties exceed the byte-size limit.
var ErrPropertiesTooLarge = errors.New("analytics: properties too large")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
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
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

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

// InvalidOptionsError reports an analytics options validation failure.
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

// CountLimitError reports properties exceeding the count limit.
type CountLimitError struct {
	// Count is the observed property count.
	Count int
	// Limit is the maximum allowed property count.
	Limit int
}

// Error returns a human-readable count-limit message.
func (e CountLimitError) Error() string {
	return fmt.Sprintf("%s: count %d exceeds limit %d", ErrTooManyProperties, e.Count, e.Limit)
}

// Unwrap returns ErrTooManyProperties.
func (e CountLimitError) Unwrap() error {
	return ErrTooManyProperties
}

// SizeLimitError reports properties exceeding the byte-size limit.
type SizeLimitError struct {
	// Size is the observed payload size in bytes.
	Size int
	// Limit is the maximum allowed payload size in bytes.
	Limit int
}

// Error returns a human-readable size-limit message.
func (e SizeLimitError) Error() string {
	return fmt.Sprintf("%s: size %d exceeds limit %d", ErrPropertiesTooLarge, e.Size, e.Limit)
}

// Unwrap returns ErrPropertiesTooLarge.
func (e SizeLimitError) Unwrap() error {
	return ErrPropertiesTooLarge
}
