package outbox

import (
	"errors"
	"fmt"
	"time"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("outbox: nil factory")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("outbox: unknown adapter")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("outbox: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("outbox: invalid adapter")

// ErrInvalidOptions is returned for invalid outbox options.
var ErrInvalidOptions = errors.New("outbox: invalid options")

// ErrInvalidMessage is returned for an invalid outbox message.
var ErrInvalidMessage = errors.New("outbox: invalid message")

// ErrTxRequired is returned when Record or Process is called without a
// transaction. The outbox write must share the business write's transaction.
var ErrTxRequired = errors.New("outbox: transaction required")

// ErrNotFound is returned when a message cannot be found.
var ErrNotFound = errors.New("outbox: not found")

// ErrStalled is returned when the relay detects that pending messages are
// not draining.
var ErrStalled = errors.New("outbox: stalled")

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
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

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

// InvalidOptionsError reports an outbox options validation failure.
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

// InvalidMessageError reports an outbox message validation failure.
type InvalidMessageError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-message message.
func (e InvalidMessageError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidMessage, e.Reason)
}

// Unwrap returns ErrInvalidMessage.
func (e InvalidMessageError) Unwrap() error { return ErrInvalidMessage }

// StalledError reports that pending messages are not draining: the oldest
// pending row is older than the stall threshold, or the relay keeps failing.
type StalledError struct {
	// Pending is the number of pending messages at detection time.
	Pending int64
	// Oldest is how long the oldest pending message has waited.
	Oldest time.Duration
	// LastError is the most recent relay or publish error, if any.
	LastError string
}

// Error returns a human-readable stalled message.
func (e StalledError) Error() string {
	return fmt.Sprintf("%s: %d pending, oldest %s, last error %q", ErrStalled, e.Pending, e.Oldest, e.LastError)
}

// Unwrap returns ErrStalled.
func (e StalledError) Unwrap() error { return ErrStalled }
