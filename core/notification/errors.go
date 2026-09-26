package notification

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the notifier is closed.
var ErrClosed = errors.New("notification: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("notification: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("notification: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("notification: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("notification: invalid adapter")

// ErrInvalidOptions is returned for invalid notifier options.
var ErrInvalidOptions = errors.New("notification: invalid options")

// ErrNilNotification is returned when a notify notification is nil.
var ErrNilNotification = errors.New("notification: nil notification")

// ErrInvalidNotification is returned for an invalid notification shape.
var ErrInvalidNotification = errors.New("notification: invalid notification")

// ErrInvalidTarget is returned for an invalid notification target.
var ErrInvalidTarget = errors.New("notification: invalid target")

// ErrInvalidChannel is returned for an invalid notification channel.
var ErrInvalidChannel = errors.New("notification: invalid channel")

// ErrChannelNotSupported is returned when an adapter does not support the channel.
var ErrChannelNotSupported = errors.New("notification: channel not supported")

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

// InvalidOptionsError reports a notifier options validation failure.
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

// InvalidNotificationError reports a notification validation failure.
type InvalidNotificationError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-notification message.
func (e InvalidNotificationError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidNotification, e.Reason)
}

// Unwrap returns ErrInvalidNotification.
func (e InvalidNotificationError) Unwrap() error { return ErrInvalidNotification }

// InvalidTargetError reports a notification target validation failure.
// It carries only a reason, never the target itself, to avoid leaking PII to logs.
type InvalidTargetError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-target message.
func (e InvalidTargetError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidTarget, e.Reason)
}

// Unwrap returns ErrInvalidTarget.
func (e InvalidTargetError) Unwrap() error { return ErrInvalidTarget }

// InvalidChannelError reports an invalid notification channel name.
type InvalidChannelError struct {
	// Channel is the invalid channel name.
	Channel string
}

// Error returns a human-readable invalid-channel message.
func (e InvalidChannelError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidChannel, e.Channel)
}

// Unwrap returns ErrInvalidChannel.
func (e InvalidChannelError) Unwrap() error { return ErrInvalidChannel }

// ChannelNotSupportedError reports a channel the adapter does not support.
type ChannelNotSupportedError struct {
	// Channel is the unsupported channel.
	Channel Channel
}

// Error returns a human-readable channel-not-supported message.
func (e ChannelNotSupportedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrChannelNotSupported, string(e.Channel))
}

// Unwrap returns ErrChannelNotSupported.
func (e ChannelNotSupportedError) Unwrap() error { return ErrChannelNotSupported }
