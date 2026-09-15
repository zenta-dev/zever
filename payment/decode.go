package payment

import (
	"bytes"
	"encoding/json"
	"io"
)

// LimitDecode decodes JSON data into v, rejecting payloads larger than limit bytes.
func LimitDecode(data []byte, v any, limit int) error {
	if limit <= 0 {
		return &SizeLimitError{Size: len(data), Limit: limit}
	}

	if len(data) > limit {
		return &SizeLimitError{Size: len(data), Limit: limit}
	}

	return json.NewDecoder(io.LimitReader(bytes.NewReader(data), int64(limit))).Decode(v)
}
