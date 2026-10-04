package codec

import (
	"encoding/json/v2"
	"errors"
	"fmt"
)

// encoding/json/v2 (see the Go 1.27 release notes for details)

// JSONCodec is a Codec[V] implementation that uses JSON encoding.
type JSONCodec[V any] struct{}

// Encode marshals v into JSON.
func (JSONCodec[V]) Encode(v V) ([]byte, error) {
	data, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("%w", errors.Join(ErrEncode, err))
	}
	return data, nil
}

// Decode unmarshals JSON data into a value of type V.
func (JSONCodec[V]) Decode(data []byte) (V, error) {
	var v V
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("%w", errors.Join(ErrDecode, err))
	}
	return v, nil
}
