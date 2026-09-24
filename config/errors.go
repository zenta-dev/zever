package config

import (
	"errors"
	"fmt"
)

// ErrUnknownService reports a service name with no registered loader.
var ErrUnknownService = errors.New("config: unknown service")

// ErrUnknownField reports a field name that a service's options do not define.
var ErrUnknownField = errors.New("config: unknown field")

// ErrUnsupportedFormat reports a config file extension this package cannot decode.
var ErrUnsupportedFormat = errors.New("config: unsupported format")

// ErrInvalidOptions reports service options that fail validation.
var ErrInvalidOptions = errors.New("config: invalid options")

// ErrDecode reports a failure to decode a service's options.
var ErrDecode = errors.New("config: decode")

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
}

// Error describes the unrecognized field and its service, with a "did you
// mean %q?" hint when Suggestion is set.
func (e *UnknownFieldError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("config: unknown field %q for service %q (did you mean %q?)", e.Field, e.Service, e.Suggestion)
	}

	return fmt.Sprintf("config: unknown field %q for service %q", e.Field, e.Service)
}

// Unwrap exposes ErrUnknownField for errors.Is.
func (e *UnknownFieldError) Unwrap() error {
	return ErrUnknownField
}

// DecodeError carries the underlying failure from decoding a service's options.
type DecodeError struct {
	// Service is the service whose options failed to decode.
	Service string
	// Err is the underlying decode failure.
	Err error
}

// Error describes which service failed to decode and why.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("config: decode %s: %v", e.Service, e.Err)
}

// Unwrap exposes the underlying decode failure for errors.Is and errors.As.
func (e *DecodeError) Unwrap() error {
	return e.Err
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
