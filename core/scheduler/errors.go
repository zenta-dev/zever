package scheduler

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("scheduler: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("scheduler: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("scheduler: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("scheduler: invalid adapter")

// ErrInvalidOptions is returned for invalid scheduler options.
var ErrInvalidOptions = errors.New("scheduler: invalid options")

// ErrInvalidSpec is returned for an invalid cron spec or entry id.
var ErrInvalidSpec = errors.New("scheduler: invalid spec")

// DuplicateAdapterError reports a duplicate adapter name.
type DuplicateAdapterError struct {
	// Adapter is the duplicate adapter name.
	Adapter string
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-adapter message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicateAdapter }

// UnknownAdapterError reports an unknown adapter name.
type UnknownAdapterError struct {
	// Adapter is the unknown adapter name.
	Adapter string
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
func (e InvalidSpecError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrInvalidSpec, e.Err)
	}

	return ErrInvalidSpec
}
