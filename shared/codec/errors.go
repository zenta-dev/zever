package codec

import (
	"errors"
	"fmt"
)

// Sentinel errors for codec operations.
var (
	// ErrEncode is returned when encoding a value fails.
	ErrEncode = errors.New("codec: encode failed")
	// ErrDecode is returned when decoding a value fails.
	ErrDecode = errors.New("codec: decode failed")
)

// EncodeError reports a failed encode. It names the type of the value that
// could not be encoded, so the developer can tell which payload shape broke
// without re-deriving it from the call site.
type EncodeError struct {
	// Type is the encoded value's type name (for example "main.User").
	Type string
	// Err is the underlying encoding cause.
	Err error
}

// Error returns a human-readable encode failure message.
func (e *EncodeError) Error() string {
	return fmt.Sprintf("codec: encode %s failed: %v", e.Type, e.Err)
}

// Unwrap returns ErrEncode joined with the cause, so errors.Is matches the
// sentinel and errors.As reaches the underlying error.
func (e *EncodeError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrEncode, e.Err)
	}
	return ErrEncode
}

// DecodeError reports a failed decode. It names the type the bytes were
// being decoded into, so the developer can tell which payload shape broke
// without re-deriving it from the call site.
type DecodeError struct {
	// Type is the target type name (for example "main.User").
	Type string
	// Err is the underlying decoding cause.
	Err error
}

// Error returns a human-readable decode failure message.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("codec: decode into %s failed: %v", e.Type, e.Err)
}

// Unwrap returns ErrDecode joined with the cause, so errors.Is matches the
// sentinel and errors.As reaches the underlying error.
func (e *DecodeError) Unwrap() error {
	if e.Err != nil {
		return errors.Join(ErrDecode, e.Err)
	}
	return ErrDecode
}
