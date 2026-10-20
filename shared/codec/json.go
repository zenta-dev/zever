package codec

import (
	"encoding/json/v2"
	"reflect"
	"time"
)

// encoding/json/v2 (see the Go 1.27 release notes for details)

// durationMarshalers renders a time.Duration as the canonical string form
// ("30s", "1m30s") that time.ParseDuration reads, which is the form the
// config env layer accepts. encoding/json/v2 has no default representation
// for time.Duration: without this marshaler every duration-bearing value
// fails to encode ("no default representation"), so a config envelope would
// drop the whole options object instead of showing the timeout.
var durationMarshalers = json.MarshalFunc(func(d time.Duration) ([]byte, error) {
	return json.Marshal(d.String())
})

// durationUnmarshalers parses that string form back into a time.Duration, so
// Encode and Decode round-trip. A JSON number is read as nanoseconds, the
// shape encoding/json v1 produced, and null reads as the zero duration.
var durationUnmarshalers = json.UnmarshalFunc(func(data []byte, d *time.Duration) error {
	var text *string
	if err := json.Unmarshal(data, &text); err == nil {
		if text == nil {
			*d = 0

			return nil
		}

		parsed, perr := time.ParseDuration(*text)
		if perr != nil {
			return perr
		}

		*d = parsed

		return nil
	}

	var nanos int64
	if err := json.Unmarshal(data, &nanos); err != nil {
		return err
	}

	*d = time.Duration(nanos)

	return nil
})

// JSONCodec is a Codec[V] implementation that uses JSON encoding.
type JSONCodec[V any] struct{}

// Encode marshals v into JSON.
//
// A failure names the value's type, so a caller can tell which payload
// shape broke without re-deriving it from the call site.
func (JSONCodec[V]) Encode(v V) ([]byte, error) {
	data, err := json.Marshal(v, json.Deterministic(true), json.WithMarshalers(durationMarshalers))
	if err != nil {
		return nil, &EncodeError{Type: encodeTypeName(v), Err: err}
	}
	return data, nil
}

// Decode unmarshals JSON data into a value of type V.
//
// A failure names the target type, so a caller can tell which payload shape
// broke without re-deriving it from the call site.
func (JSONCodec[V]) Decode(data []byte) (V, error) {
	var v V
	if err := json.Unmarshal(data, &v, json.WithUnmarshalers(durationUnmarshalers)); err != nil {
		return v, &DecodeError{Type: decodeTypeName[V](), Err: err}
	}
	return v, nil
}

// encodeTypeName renders the concrete type of an encoded value, falling back
// to the codec's declared type for a nil value (which would otherwise render
// as "<nil>").
func encodeTypeName[V any](v V) string {
	if t := reflect.TypeOf(v); t != nil {
		return t.String()
	}

	return reflect.TypeFor[V]().String()
}

// decodeTypeName renders the codec's declared type: the bytes are being
// decoded into that type, whatever the decoder managed to fill in before it
// failed.
func decodeTypeName[V any]() string {
	return reflect.TypeFor[V]().String()
}
