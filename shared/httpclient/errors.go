package httpclient

import (
	"errors"
	"fmt"
)

// ErrTooLarge is returned (via TooLargeError) when ReadLimited exceeds limit.
var ErrTooLarge = errors.New("httpclient: response body too large")

// ErrPrivateAddress is returned when SafeDialContext resolves a private
// address and allowPrivate is false.
var ErrPrivateAddress = errors.New("httpclient: refusing to dial private address")

// ErrNoAddresses is returned when SafeDialContext has no addresses to dial.
var ErrNoAddresses = errors.New("httpclient: no addresses to dial")

// TooLargeError reports a body exceeding a byte limit.
type TooLargeError struct {
	// Limit is the maximum allowed bytes.
	Limit int64
	// Size is the bytes observed (capped at Limit+1).
	Size int64
}

// Error returns a human-readable description of the limit breach.
func (e *TooLargeError) Error() string {
	return fmt.Sprintf("httpclient: response body too large: %d bytes exceeds limit %d", e.Size, e.Limit)
}

// Is reports whether target is ErrTooLarge.
func (e *TooLargeError) Is(target error) bool {
	return target == ErrTooLarge
}

// Unwrap exposes ErrTooLarge for errors.Is and errors.As.
func (e *TooLargeError) Unwrap() error {
	return ErrTooLarge
}
