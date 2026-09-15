package document

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("document: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("document: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("document: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("document: invalid adapter")

// ErrInvalidOptions is returned for invalid document options.
var ErrInvalidOptions = errors.New("document: invalid options")

// ErrUnsupportedFormat is returned for an unsupported output format.
var ErrUnsupportedFormat = errors.New("document: unsupported format")

// ErrSourceTooLarge is returned when a document source exceeds the size limit.
var ErrSourceTooLarge = errors.New("document: source too large")

// ErrMissingEndpoint is returned when a remote operation requires an endpoint but none is configured.
var ErrMissingEndpoint = errors.New("document: missing endpoint")

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
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter)
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

// InvalidOptionsError reports a document options validation failure.
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

// UnsupportedFormatError reports an unsupported output format.
type UnsupportedFormatError struct {
	// Format is the unsupported output format.
	Format OutputFormat
}

// Error returns a human-readable unsupported-format message.
func (e UnsupportedFormatError) Error() string {
	return fmt.Sprintf("%s: %q (want pdf, png, or jpg)", ErrUnsupportedFormat, e.Format)
}

// Unwrap returns ErrUnsupportedFormat.
func (e UnsupportedFormatError) Unwrap() error {
	return ErrUnsupportedFormat
}

// SizeLimitError reports a document source exceeding the size limit.
type SizeLimitError struct {
	// Size is the observed source size in bytes.
	Size int
	// Limit is the maximum allowed source size in bytes.
	Limit int
}

// Error returns a human-readable size-limit message.
func (e SizeLimitError) Error() string {
	return fmt.Sprintf("%s: size %d exceeds limit %d", ErrSourceTooLarge, e.Size, e.Limit)
}

// Unwrap returns ErrSourceTooLarge.
func (e SizeLimitError) Unwrap() error {
	return ErrSourceTooLarge
}
