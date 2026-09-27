package redisopt

import (
	"errors"
	"fmt"
)

// Sentinel errors returned and wrapped by the redis packages.
var (
	ErrInvalidAddress    = errors.New("redis: invalid address")
	ErrParseAddress      = errors.New("redis: parse address failed")
	ErrCloseClient       = errors.New("redis: close client failed")
	ErrPlaintextRejected = errors.New("redis: RequireTLS is set but the connection would be plaintext")
)

// InvalidAddressError describes a failure to validate or parse a Redis address.
type InvalidAddressError struct {
	// Addr holds the invalid Redis address that caused the error.
	Addr string
	// Err holds the underlying parsing or validation error, if any.
	Err error
}

// Error returns a formatted description of the invalid address.
func (e *InvalidAddressError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s %q: %v", ErrInvalidAddress.Error(), e.Addr, e.Err)
	}
	return fmt.Sprintf("%s %q", ErrInvalidAddress.Error(), e.Addr)
}

// Unwrap returns the sentinel and underlying errors for error inspection.
func (e *InvalidAddressError) Unwrap() []error {
	if e.Err != nil {
		return []error{ErrInvalidAddress, e.Err}
	}
	return []error{ErrInvalidAddress}
}

// PlaintextRejectedError reports that RequireTLS is set on Options but the
// resolved address would connect without TLS.
type PlaintextRejectedError struct {
	// Addr holds the address that would have connected in plaintext.
	Addr string
}

// Error returns a formatted description of the rejected plaintext connection.
func (e *PlaintextRejectedError) Error() string {
	return fmt.Sprintf("%s %q", ErrPlaintextRejected.Error(), e.Addr)
}

// Unwrap returns the sentinel error for error inspection.
func (e *PlaintextRejectedError) Unwrap() error {
	return ErrPlaintextRejected
}
