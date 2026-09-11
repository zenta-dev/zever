package codec

import "errors"

// Sentinel errors for codec operations.
var (
	ErrEncode = errors.New("codec: encode failed")
	ErrDecode = errors.New("codec: decode failed")
)
