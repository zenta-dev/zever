package payment

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when a nil backend factory is provided.
var ErrNilFactory = errors.New("payment: nil factory")

// ErrDuplicateAdapter is returned for a duplicate adapter name.
var ErrDuplicateAdapter = errors.New("payment: duplicate adapter")

// ErrDuplicate aliases ErrDuplicateAdapter for compatibility.
var ErrDuplicate = ErrDuplicateAdapter

// ErrUnknownAdapter is returned for an unknown adapter name.
var ErrUnknownAdapter = errors.New("payment: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("payment: invalid adapter")

// ErrInvalidOptions is returned for invalid payment options.
var ErrInvalidOptions = errors.New("payment: invalid options")

// ErrInvalidAmount is returned when a payment amount is not positive.
var ErrInvalidAmount = errors.New("payment: amount must be positive")

// ErrMissingCurrency is returned when a payment request is missing the currency.
var ErrMissingCurrency = errors.New("payment: missing currency")

// ErrUnsupportedMethod is returned for an unsupported payment method.
var ErrUnsupportedMethod = errors.New("payment: unsupported method")

// ErrNotFound is returned when a payment cannot be found.
var ErrNotFound = errors.New("payment: payment not found")

// ErrMissingPaymentID is returned when an operation requires a payment ID but none is present.
var ErrMissingPaymentID = errors.New("payment: missing payment id")

// ErrWebhookTooLarge is returned when a webhook payload exceeds the size limit.
var ErrWebhookTooLarge = errors.New("payment: webhook too large")

// ErrMissingWebhookSecret is returned when a webhook operation requires a secret but none is configured.
var ErrMissingWebhookSecret = errors.New("payment: missing webhook secret")

// ErrInvalidSignature is returned for an invalid webhook signature.
var ErrInvalidSignature = errors.New("payment: invalid signature")

// ErrDuplicateEvent is returned when a webhook event with an already-seen
// provider event id is delivered again. Callers should treat it as
// already-processed and acknowledge the delivery without re-executing.
var ErrDuplicateEvent = errors.New("payment: duplicate webhook event")

// ErrAmountMismatch is returned when a payment amount does not match the expected amount.
var ErrAmountMismatch = errors.New("payment: amount mismatch")

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
func (e DuplicateAdapterError) Unwrap() error {
	return ErrDuplicateAdapter
}

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

// InvalidOptionsError reports a payment options validation failure.
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

// NotFoundError reports a lookup of a payment that does not exist.
type NotFoundError struct {
	// PaymentID is the payment identifier that was not found.
	PaymentID string
}

// Error returns a human-readable payment-not-found message.
func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s: %q", ErrNotFound, e.PaymentID)
}

// Unwrap returns ErrNotFound.
func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}

// UnsupportedMethodError reports an unsupported payment method.
type UnsupportedMethodError struct {
	// Method is the unsupported payment method.
	Method PaymentMethod
}

// Error returns a human-readable unsupported-method message.
func (e UnsupportedMethodError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnsupportedMethod, e.Method)
}

// Unwrap returns ErrUnsupportedMethod.
func (e UnsupportedMethodError) Unwrap() error {
	return ErrUnsupportedMethod
}

// AmountMismatchError reports a payment amount that does not match the expected amount.
type AmountMismatchError struct {
	// Expected is the expected payment amount in minor units.
	Expected int64
	// Actual is the observed payment amount in minor units.
	Actual int64
}

// Error returns a human-readable amount-mismatch message.
func (e AmountMismatchError) Error() string {
	return fmt.Sprintf("%s: expected %d, got %d", ErrAmountMismatch, e.Expected, e.Actual)
}

// Unwrap returns ErrAmountMismatch.
func (e AmountMismatchError) Unwrap() error {
	return ErrAmountMismatch
}

// DuplicateEventError reports delivery of an already-processed webhook event.
type DuplicateEventError struct {
	// EventID is the provider-assigned id of the repeated event.
	EventID string
}

// Error returns a human-readable duplicate-event message.
func (e DuplicateEventError) Error() string {
	return fmt.Sprintf("%s: %q", ErrDuplicateEvent, e.EventID)
}

// Unwrap returns ErrDuplicateEvent.
func (e DuplicateEventError) Unwrap() error {
	return ErrDuplicateEvent
}

// SizeLimitError reports a webhook payload exceeding the size limit.
type SizeLimitError struct {
	// Size is the observed payload size in bytes.
	Size int
	// Limit is the maximum allowed payload size in bytes.
	Limit int
}

// Error returns a human-readable size-limit message.
func (e SizeLimitError) Error() string {
	return fmt.Sprintf("%s: size %d exceeds limit %d", ErrWebhookTooLarge, e.Size, e.Limit)
}

// Unwrap returns ErrWebhookTooLarge.
func (e SizeLimitError) Unwrap() error {
	return ErrWebhookTooLarge
}
