package mailer

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the mailer is closed.
var ErrClosed = errors.New("mailer: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("mailer: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("mailer: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("mailer: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("mailer: invalid adapter")

// ErrInvalidOptions is returned for invalid mailer options.
var ErrInvalidOptions = errors.New("mailer: invalid options")

// ErrInvalidAddress is returned for an invalid mail address.
var ErrInvalidAddress = errors.New("mailer: invalid address")

// ErrNoRecipients is returned when a message has no recipients.
var ErrNoRecipients = errors.New("mailer: no recipients")

// ErrMessageTooLarge is returned when a message exceeds the size limit.
var ErrMessageTooLarge = errors.New("mailer: message too large")

// ErrNilMessage is returned when a send message is nil.
var ErrNilMessage = errors.New("mailer: nil message")

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

// InvalidOptionsError reports a mailer options validation failure.
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

// InvalidAddressError reports a mail address validation failure.
type InvalidAddressError struct {
	// Field is the address field that failed validation.
	Field string
	// Value is the invalid value.
	Value string
}

// Error returns a human-readable invalid-address message.
func (e InvalidAddressError) Error() string {
	return fmt.Sprintf("%s: %s %q", ErrInvalidAddress, e.Field, e.Value)
}

// Unwrap returns ErrInvalidAddress.
func (e InvalidAddressError) Unwrap() error { return ErrInvalidAddress }
