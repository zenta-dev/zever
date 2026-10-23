package payment

import (
	"bytes"
	"io"

	"github.com/zenta-dev/zever/shared/codec"
)

// LimitDecode decodes JSON data into v, rejecting payloads larger than limit bytes.
// Size-limited decoding uses an io.LimitReader over the codec facade's
// single-value reader.
func LimitDecode(data []byte, v any, limit int) error {
	if limit <= 0 {
		return SizeLimitError{Size: len(data), Limit: limit}
	}

	if len(data) > limit {
		return SizeLimitError{Size: len(data), Limit: limit}
	}

	return codec.UnmarshalRead(io.LimitReader(bytes.NewReader(data), int64(limit)), v)
}
