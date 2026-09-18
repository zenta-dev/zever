package codec

import "errors"

// Sentinel errors for codec operations.
var (
	// ErrEncode is returned when encoding a value fails.
	ErrEncode = errors.New("codec: encode failed")
	// ErrDecode is returned when decoding a value fails.
	ErrDecode = errors.New("codec: decode failed")
)
