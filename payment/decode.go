package payment

import (
	"bytes"
	"encoding/json"
	"io"
)

// LimitDecode decodes JSON data into v, rejecting payloads larger than limit bytes.
// It intentionally stays on raw encoding/json (not codec.Codec[V]) because it
// needs an io.LimitReader for size-limited decoding, which codec.Codec[V] cannot express.
func LimitDecode(data []byte, v any, limit int) error {
	if limit <= 0 {
		return &SizeLimitError{Size: len(data), Limit: limit}
	}

	if len(data) > limit {
		return &SizeLimitError{Size: len(data), Limit: limit}
	}

	return json.NewDecoder(io.LimitReader(bytes.NewReader(data), int64(limit))).Decode(v)
}
