package codec

import (
	"encoding/json/v2"
	"fmt"
)

// encoding/json/v2 (see the Go 1.27 release notes for details)

// JSONCodec is a Codec[V] implementation that uses JSON encoding.
type JSONCodec[V any] struct{}

// Encode marshals v into JSON.
func (JSONCodec[V]) Encode(v V) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEncode, err)
	}
	return data, nil
}

// Decode unmarshals JSON data into a value of type V.
func (JSONCodec[V]) Decode(data []byte) (V, error) {
	var v V
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	return v, nil
}
