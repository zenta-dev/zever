package billing

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("billing: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("billing: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("billing: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("billing: invalid adapter")

// ErrInvalidOptions is returned for invalid billing options.
var ErrInvalidOptions = errors.New("billing: invalid options")

// ErrNotFound is returned when a billing resource cannot be found.
var ErrNotFound = errors.New("billing: not found")

// ErrMissingCustomerID is returned when an operation requires a customer ID but none is present.
var ErrMissingCustomerID = errors.New("billing: missing customer id")

// ErrMissingPlanID is returned when an operation requires a plan ID but none is present.
var ErrMissingPlanID = errors.New("billing: missing plan id")

// ErrMissingSubscriptionID is returned when an operation requires a subscription ID but none is present.
var ErrMissingSubscriptionID = errors.New("billing: missing subscription id")

// ErrEmptyAmount is returned when an amount string is empty.
var ErrEmptyAmount = errors.New("billing: empty amount")

// ErrMalformedAmount is returned when an amount string is malformed.
var ErrMalformedAmount = errors.New("billing: malformed amount")

// ErrAmountOverflow is returned when an amount overflows int64.
var ErrAmountOverflow = errors.New("billing: amount overflow")

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

// InvalidOptionsError reports a billing options validation failure.
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

// NotFoundError reports a lookup of a billing resource that does not exist.
type NotFoundError struct {
	// Resource is the resource type that was not found.
	Resource string
	// ID is the resource identifier that was not found.
	ID string
}

// Error returns a human-readable resource-not-found message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s: %s %q not found", ErrNotFound, e.Resource, e.ID)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}
