package ai

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNilFactory is returned when an adapter factory is nil.
	ErrNilFactory = errors.New("ai: nil factory")
	// ErrDuplicateAdapter is returned on duplicate adapter registration.
	ErrDuplicateAdapter = errors.New("ai: duplicate adapter")
	// ErrUnknownAdapter is returned for an unregistered adapter.
	ErrUnknownAdapter = errors.New("ai: unknown adapter")
	// ErrInvalidAdapter is returned for an invalid adapter name.
	ErrInvalidAdapter = errors.New("ai: invalid adapter")
	// ErrInvalidOptions is returned for invalid options.
	ErrInvalidOptions = errors.New("ai: invalid options")
	// ErrModelNotFound is returned when the requested model does not exist.
	ErrModelNotFound = errors.New("ai: model not found")
	// ErrNotSupported is returned when an operation is not supported.
	ErrNotSupported = errors.New("ai: not supported")
	// ErrInvalidRequest is returned when a generation request is invalid.
	ErrInvalidRequest = errors.New("ai: invalid request")
	// ErrAuth is returned on authentication failure.
	ErrAuth = errors.New("ai: auth failed")
	// ErrRateLimited is returned when the provider rate-limits the request.
	ErrRateLimited = errors.New("ai: rate limited")
)

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e *InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e *InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e *DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicateAdapter, e.Adapter)
}

// Unwrap returns ErrDuplicateAdapter.
func (e *DuplicateAdapterError) Unwrap() error {
	return ErrDuplicateAdapter
}

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e *UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownAdapter, e.Adapter)
}

// Unwrap returns ErrUnknownAdapter.
func (e *UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

// InvalidOptionsError reports an options validation failure.
type InvalidOptionsError struct {
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e *InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e *InvalidOptionsError) Unwrap() error {
	return ErrInvalidOptions
}

// RateLimitedError reports a provider rate-limit with optional retry hint.
type RateLimitedError struct {
	RetryAfter time.Duration
}

// Error returns a human-readable rate-limited message.
func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s: retry after %s", ErrRateLimited, e.RetryAfter)
	}
	return ErrRateLimited.Error()
}

// Unwrap returns ErrRateLimited.
func (e *RateLimitedError) Unwrap() error {
	return ErrRateLimited
}
