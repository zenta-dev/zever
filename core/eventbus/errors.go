package eventbus

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the eventbus is closed.
var ErrClosed = errors.New("eventbus: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("eventbus: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("eventbus: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("eventbus: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("eventbus: invalid adapter")

// ErrInvalidOptions is returned for invalid eventbus options.
var ErrInvalidOptions = errors.New("eventbus: invalid options")

// ErrNilHandler is returned when a subscribe handler is nil.
var ErrNilHandler = errors.New("eventbus: nil handler")

// ErrPayloadTooLarge is returned when a message payload exceeds the size limit.
var ErrPayloadTooLarge = errors.New("eventbus: payload too large")

// ErrInvalidMessageID is returned for an invalid message ID.
var ErrInvalidMessageID = errors.New("eventbus: invalid message id")

// ErrNotSubscribed is returned when unsubscribing a topic that has no
// matching pull subscription.
var ErrNotSubscribed = errors.New("eventbus: not subscribed")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicate }

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

// InvalidOptionsError reports an eventbus options validation failure.
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

// InvalidMessageIDError reports a failure to parse a message ID.
type InvalidMessageIDError struct {
	// ID is the invalid message ID string.
	ID string
	// Err is the underlying parse cause.
	Err error
}

// Error returns a human-readable invalid-message-ID message.
func (e InvalidMessageIDError) Error() string {
	return fmt.Sprintf("%s %q: %v", ErrInvalidMessageID, e.ID, e.Err)
}

// Unwrap returns ErrInvalidMessageID and the cause for errors.Is/As.
func (e InvalidMessageIDError) Unwrap() []error {
	if e.Err != nil {
		return []error{ErrInvalidMessageID, e.Err}
	}
	return []error{ErrInvalidMessageID}
}
