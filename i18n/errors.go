package i18n

import (
	"errors"
	"fmt"
)

// ErrClosed is returned when the backend is closed.
var ErrClosed = errors.New("i18n: closed")

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("i18n: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("i18n: duplicate registration")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("i18n: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("i18n: invalid adapter")

// ErrInvalidOptions is returned for invalid backend options.
var ErrInvalidOptions = errors.New("i18n: invalid options")

// ErrLocaleNotFound is returned when a locale has no catalog.
var ErrLocaleNotFound = errors.New("i18n: locale not found")

// ErrKeyNotFound is returned when a message key is missing.
var ErrKeyNotFound = errors.New("i18n: key not found")

// ErrRemoteError is returned when the remote service fails.
var ErrRemoteError = errors.New("i18n: remote error")

// DuplicateError reports a duplicate adapter registration.
type DuplicateError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e DuplicateError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter.String())
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

// InvalidOptionsError reports a backend options validation failure.
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

// LocaleNotFoundError reports a missing locale catalog.
// It echoes the locale, which is not PII.
type LocaleNotFoundError struct {
	// Locale is the missing locale.
	Locale string
}

// Error returns a human-readable locale-not-found message.
func (e LocaleNotFoundError) Error() string {
	return fmt.Sprintf("%s: %s", ErrLocaleNotFound, e.Locale)
}

// Unwrap returns ErrLocaleNotFound.
func (e LocaleNotFoundError) Unwrap() error { return ErrLocaleNotFound }

// KeyNotFoundError reports a missing message key.
type KeyNotFoundError struct {
	// Locale is the lookup locale.
	Locale string
	// Key is the missing message key.
	Key string
}

// Error returns a human-readable key-not-found message.
func (e KeyNotFoundError) Error() string {
	return fmt.Sprintf("%s: %s/%s", ErrKeyNotFound, e.Locale, e.Key)
}

// Unwrap returns ErrKeyNotFound.
func (e KeyNotFoundError) Unwrap() error { return ErrKeyNotFound }
