package redisopt

import (
	"errors"
	"fmt"
)

// ErrInvalidAddress indicates an invalid Redis address.
var ErrInvalidAddress = errors.New("redis: invalid address")

// ErrParseAddress indicates a Redis address parse failure.
var ErrParseAddress = errors.New("redis: parse address")

// ErrCloseClient indicates a Redis client close failure.
var ErrCloseClient = errors.New("redis: close client")

// ErrPlaintextRejected indicates a plaintext connection was rejected.
var ErrPlaintextRejected = errors.New("redis: plaintext rejected")

// InvalidAddressError describes a failure to validate or parse a Redis address.
type InvalidAddressError struct {
	// Addr holds the invalid Redis address that caused the error.
	Addr string
	// Err holds the underlying parsing or validation error, if any.
	Err error
}

// Error returns a formatted description of the invalid address.
//
// The address is masked with RedactAddr before it is echoed: a rejected
// address may be a redis:// URL carrying a password, and error text must
// never leak one.
func (e *InvalidAddressError) Error() string {
	addr := RedactAddr(e.Addr)

	if e.Err != nil {
		return fmt.Sprintf("%s %q: %v", ErrInvalidAddress.Error(), addr, e.Err)
	}
	return fmt.Sprintf("%s %q", ErrInvalidAddress.Error(), addr)
}

// Unwrap returns the sentinel joined with the underlying error for error
// inspection.
func (e *InvalidAddressError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrInvalidAddress, e.Err)
	}
	return ErrInvalidAddress
}

// PlaintextRejectedError reports that RequireTLS is set on Options but the
// resolved address would connect without TLS.
type PlaintextRejectedError struct {
	// Addr holds the address that would have connected in plaintext.
	Addr string
}

// Error returns a formatted description of the rejected plaintext connection.
//
// It names the address that would have been dialled in plaintext (masked by
// RedactAddr) and the two ways to turn TLS on, so the developer can fix the
// option without reading the adapter's source.
func (e *PlaintextRejectedError) Error() string {
	return fmt.Sprintf("%s %q: set tls, or use a rediss:// address, to connect over TLS",
		ErrPlaintextRejected.Error(), RedactAddr(e.Addr))
}

// Unwrap returns the sentinel error for error inspection.
func (e *PlaintextRejectedError) Unwrap() error {
	return ErrPlaintextRejected
}
