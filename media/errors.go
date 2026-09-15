package media

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("media: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("media: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("media: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("media: invalid adapter")

// ErrInvalidOptions is returned for invalid media options.
var ErrInvalidOptions = errors.New("media: invalid options")

// ErrNotFound is returned when a media asset cannot be found.
var ErrNotFound = errors.New("media: asset not found")

// ErrInvalidID is returned for an invalid asset identifier.
var ErrInvalidID = errors.New("media: invalid id")

// ErrTooLarge is returned when an asset exceeds the size limit.
var ErrTooLarge = errors.New("media: too large")

// ErrUnsupportedFormat is returned for an unsupported media format.
var ErrUnsupportedFormat = errors.New("media: unsupported format")

// ErrInvalidTransform is returned for an invalid transform request.
var ErrInvalidTransform = errors.New("media: invalid transform")

// ErrInvalidRange is returned for an invalid download range.
var ErrInvalidRange = errors.New("media: invalid range")

// ErrDurationExceeded is returned when media exceeds the duration limit.
var ErrDurationExceeded = errors.New("media: duration exceeded")

// ErrToolMissing is returned when a required external tool is missing.
var ErrToolMissing = errors.New("media: tool missing")

// ErrProbeFailed is returned when media probing fails.
var ErrProbeFailed = errors.New("media: probe failed")

// ErrTranscodeFailed is returned when media transcoding fails.
var ErrTranscodeFailed = errors.New("media: transcode failed")

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

// InvalidOptionsError reports a media options validation failure.
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

// NotFoundError reports a lookup of an asset that does not exist.
type NotFoundError struct {
	// ID is the asset identifier that was not found.
	ID string
}

// Error returns a human-readable asset-not-found message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s: %q", ErrNotFound, e.ID)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}

// InvalidIDError reports an invalid asset identifier.
type InvalidIDError struct {
	// ID is the invalid asset identifier.
	ID string
}

// Error returns a human-readable invalid-id message.
func (e InvalidIDError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidID, e.ID)
}

// Unwrap returns ErrInvalidID.
func (e InvalidIDError) Unwrap() error {
	return ErrInvalidID
}

// SizeLimitError reports an asset exceeding the size limit.
type SizeLimitError struct {
	// Size is the observed asset size in bytes.
	Size int
	// Limit is the maximum allowed asset size in bytes.
	Limit int
}

// Error returns a human-readable size-limit message.
func (e SizeLimitError) Error() string {
	return fmt.Sprintf("%s: size %d exceeds limit %d", ErrTooLarge, e.Size, e.Limit)
}

// Unwrap returns ErrTooLarge.
func (e SizeLimitError) Unwrap() error {
	return ErrTooLarge
}

// UnsupportedFormatError reports an unsupported media format.
type UnsupportedFormatError struct {
	// Format is the unsupported media format.
	Format string
}

// Error returns a human-readable unsupported-format message.
func (e UnsupportedFormatError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnsupportedFormat, e.Format)
}

// Unwrap returns ErrUnsupportedFormat.
func (e UnsupportedFormatError) Unwrap() error {
	return ErrUnsupportedFormat
}

// InvalidTransformError reports an invalid transform request.
type InvalidTransformError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-transform message.
func (e InvalidTransformError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidTransform, e.Reason)
}

// Unwrap returns ErrInvalidTransform.
func (e InvalidTransformError) Unwrap() error {
	return ErrInvalidTransform
}

// InvalidRangeError reports an invalid download range.
type InvalidRangeError struct {
	// Offset is the requested range start in bytes.
	Offset int64
	// Length is the requested range length in bytes.
	Length int64
	// Size is the asset size in bytes.
	Size int64
}

// Error returns a human-readable invalid-range message.
func (e InvalidRangeError) Error() string {
	return fmt.Sprintf("%s: offset %d length %d size %d", ErrInvalidRange, e.Offset, e.Length, e.Size)
}

// Unwrap returns ErrInvalidRange.
func (e InvalidRangeError) Unwrap() error {
	return ErrInvalidRange
}
