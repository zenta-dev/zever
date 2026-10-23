package crypto

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidKey is returned when a cryptographic key is invalid.
	ErrInvalidKey = errors.New("crypto: invalid key")
	// ErrKeyNotFound is returned when a requested key is not found.
	ErrKeyNotFound = errors.New("crypto: key not found")
	// ErrNotSupported is returned when an operation is not supported by the adapter.
	ErrNotSupported = errors.New("crypto: not supported")
	// ErrIntegrity is returned when an integrity check fails.
	ErrIntegrity = errors.New("crypto: integrity check failed")
	// ErrNilFactory is returned when an adapter factory is nil.
	ErrNilFactory = errors.New("crypto: nil factory")
	// ErrDuplicateAdapter is returned on duplicate adapter registration.
	ErrDuplicateAdapter = errors.New("crypto: duplicate adapter")
	// ErrUnknownAdapter is returned for an unregistered adapter.
	ErrUnknownAdapter = errors.New("crypto: unknown adapter")
	// ErrInvalidAdapter is returned for an invalid adapter name.
	ErrInvalidAdapter = errors.New("crypto: invalid adapter")
	// ErrInvalidOptions is returned for invalid crypto options.
	ErrInvalidOptions = errors.New("crypto: invalid options")
	// ErrDevKey is returned when the deterministic public dev key is used
	// without an explicit allow (Options.AllowDevKey or a dev ZEVER_ENV).
	ErrDevKey = errors.New("crypto: dev key not allowed")
)

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

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

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter string
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
	Adapter string
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidOptionsError reports a crypto options validation failure.
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

// DevKeyError reports a rejected deterministic dev key: the public
// DevCryptoKey was supplied without an explicit allow. The message carries
// the fix hint; branch with errors.Is(err, ErrDevKey) or errors.As for
// the typed form.
type DevKeyError struct{}

// Error returns the rejection message with the fix hint.
func (e DevKeyError) Error() string {
	return "crypto: refusing the deterministic public dev key: set a real crypto key, or set allow_dev_key: true or ZEVER_ENV=dev|test|development for local development"
}

// Unwrap returns ErrDevKey.
func (e DevKeyError) Unwrap() error { return ErrDevKey }
