package pretty

import (
	"errors"
	"fmt"
)

// Sentinel errors for pretty option handling.
var (
	// ErrUnknownOption is returned for an unrecognized option key.
	ErrUnknownOption = errors.New("pretty: unknown option")
	// ErrInvalidOption is returned for a wrongly-typed option value.
	ErrInvalidOption = errors.New("pretty: invalid option")
)

// UnknownOptionError reports an unrecognized option key.
type UnknownOptionError struct {
	// Option is the unrecognized key.
	Option string
}

// Error returns a human-readable unknown-option message.
func (e *UnknownOptionError) Error() string {
	return fmt.Sprintf("%s: %q", ErrUnknownOption.Error(), e.Option)
}

// Unwrap returns ErrUnknownOption for errors.Is matching.
func (e *UnknownOptionError) Unwrap() error { return ErrUnknownOption }

// InvalidOptionError reports a wrongly-typed option value.
type InvalidOptionError struct {
	// Option is the offending key.
	Option string
}

// Error returns a human-readable invalid-option message.
func (e *InvalidOptionError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidOption.Error(), e.Option)
}

// Unwrap returns ErrInvalidOption for errors.Is matching.
func (e *InvalidOptionError) Unwrap() error { return ErrInvalidOption }
