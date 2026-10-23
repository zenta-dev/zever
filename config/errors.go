package config

import (
	"errors"
	"fmt"
)

// ErrUnknownService reports a service name with no registered loader.
var ErrUnknownService = errors.New("config: unknown service")

// ErrUnknownField reports a field name that a service's options do not define.
var ErrUnknownField = errors.New("config: unknown field")

// ErrUnknownAdapter reports an adapter name with no registered factory.
var ErrUnknownAdapter = errors.New("config: unknown adapter")

// ErrInvalidOptions reports service options that fail validation.
var ErrInvalidOptions = errors.New("config: invalid options")

// ErrInvalidPlugin indicates a plugin validator is invalid.
var ErrInvalidPlugin = errors.New("config: invalid plugin")

// UnknownServiceError names a service with no registered loader.
type UnknownServiceError struct {
	// Service is the unrecognized service name.
	Service string
	// Suggestion is the closest known service name, if any is close
	// enough to be worth suggesting (see closest in suggest.go). Empty
	// when nothing is close.
	Suggestion string
}

// Error describes the unrecognized service, with a "did you mean %q?" hint
// when Suggestion is set.
func (e *UnknownServiceError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("config: unknown service %q (did you mean %q?)", e.Service, e.Suggestion)
	}

	return fmt.Sprintf("config: unknown service %q", e.Service)
}

// Unwrap exposes ErrUnknownService for errors.Is.
func (e *UnknownServiceError) Unwrap() error {
	return ErrUnknownService
}

// UnknownFieldError names a field that a service's options do not define.
type UnknownFieldError struct {
	// Service is the service the field was set on.
	Service string
	// Field is the unrecognized field name.
	Field string
	// Suggestion is the closest known field name for this service, if any
	// is close enough to be worth suggesting (see closest in suggest.go).
	// Empty when nothing is close, or when no candidate list was known at
	// the failure site.
	Suggestion string
	// Reason carries a caller-supplied explanation appended to the message
	// after the field naming. It is set instead of Suggestion when the key
	// is rejected for a reason rather than a typo, for example an
	// adapter-selection key that the environment no longer accepts.
	Reason string
}

// Error describes the unrecognized field and its service, with a "did you
// mean %q?" hint when Suggestion is set and a ": <reason>" suffix when
// Reason is set.
func (e *UnknownFieldError) Error() string {
	msg := fmt.Sprintf("config: unknown field %q for service %q", e.Field, e.Service)
	if e.Suggestion != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", e.Suggestion)
	}
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// Unwrap exposes ErrUnknownField for errors.Is.
func (e *UnknownFieldError) Unwrap() error {
	return ErrUnknownField
}

// UnknownAdapterError names a configured adapter with no registered
// factory in the process.
type UnknownAdapterError struct {
	// Service is the service the unknown adapter was configured on.
	Service string
	// Adapter is the unrecognized adapter name.
	Adapter string
	// Suggestion is the closest registered adapter name, if any is close
	// enough to be worth suggesting (see closest in suggest.go). Empty
	// when nothing is close, or when nothing is registered at all.
	Suggestion string
}

// Error describes the unrecognized adapter and its service, with a "did you
// mean %q?" hint when Suggestion is set.
func (e *UnknownAdapterError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("config: unknown adapter %q for service %q (did you mean %q?)", e.Adapter, e.Service, e.Suggestion)
	}

	return fmt.Sprintf("config: unknown adapter %q for service %q", e.Adapter, e.Service)
}

// Unwrap exposes ErrUnknownAdapter for errors.Is.
func (e *UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}

// InvalidOptionsError carries the validation reason for a service's options.
type InvalidOptionsError struct {
	// Service is the service whose options failed validation.
	Service string
	// Reason explains why the options are invalid.
	Reason string
}

// Error describes which service has invalid options and why.
func (e *InvalidOptionsError) Error() string {
	return fmt.Sprintf("config: invalid options %s: %s", e.Service, e.Reason)
}

// Unwrap exposes ErrInvalidOptions for errors.Is.
func (e *InvalidOptionsError) Unwrap() error {
	return ErrInvalidOptions
}
