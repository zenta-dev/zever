package payment

import (
	"errors"
	"fmt"
)

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("payment: nil factory")

// ErrDuplicateAdapter is returned on duplicate adapter registration.
var ErrDuplicateAdapter = errors.New("payment: duplicate adapter")

// ErrUnknownAdapter is returned for an unregistered adapter.
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

// ErrAmountMismatch is returned when a payment amount does not match the expected amount.
var ErrAmountMismatch = errors.New("payment: amount mismatch")

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
