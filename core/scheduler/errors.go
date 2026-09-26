package scheduler

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("scheduler: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("scheduler: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("scheduler: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("scheduler: invalid adapter")

// ErrInvalidOptions is returned for invalid scheduler options.
var ErrInvalidOptions = errors.New("scheduler: invalid options")

// ErrInvalidSpec is returned for an invalid cron spec or entry id.
var ErrInvalidSpec = errors.New("scheduler: invalid spec")

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

// InvalidOptionsError reports a scheduler options validation failure.
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

// InvalidSpecError reports a cron spec validation failure.
type InvalidSpecError struct {
	// Spec is the invalid cron spec.
	Spec string
	// Err is the underlying parse cause.
	Err error
}

// Error returns a human-readable invalid-spec message.
func (e InvalidSpecError) Error() string {
	return fmt.Sprintf("%s %q: %v", ErrInvalidSpec, e.Spec, e.Err)
}

// Unwrap returns ErrInvalidSpec and the cause for errors.Is/As.
func (e InvalidSpecError) Unwrap() []error {
	if e.Err != nil {
		return []error{ErrInvalidSpec, e.Err}
	}

	return []error{ErrInvalidSpec}
}
