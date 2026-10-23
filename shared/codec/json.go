package codec

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
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

// Centralized option sets for the facade.
// Adjusting these swaps behavior repo-wide without touching call sites.
var (
	// defaultMarshalOpts applies Deterministic (sorted maps) + Duration string form.
	defaultMarshalOpts = json.JoinOptions(json.Deterministic(true), json.WithMarshalers(durationMarshalers))

	// defaultUnmarshalOpts applies Duration string/number round-trip.
	defaultUnmarshalOpts = json.JoinOptions(json.WithUnmarshalers(durationUnmarshalers))

	// indentOpts for MarshalIndent (2-space indent, multiline).
	indentOpts = json.JoinOptions(jsontext.Multiline(true), jsontext.WithIndent("  "))
)

// JSONCodec is a Codec[V] implementation that uses JSON encoding.
type JSONCodec[V any] struct{}

// Encode marshals v into JSON.
//
// A failure names the value's type, so a caller can tell which payload
// shape broke without re-deriving it from the call site.
func (JSONCodec[V]) Encode(v V) ([]byte, error) {
	data, err := json.Marshal(v, defaultMarshalOpts)
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
	if err := json.Unmarshal(data, &v, defaultUnmarshalOpts); err != nil {
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

// Marshal serializes v to JSON bytes with repo-default options
// (Deterministic + Duration string form).
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v, defaultMarshalOpts)
}

// MarshalWrite serializes v to w with repo-default options.
// Does NOT append a trailing newline (unlike v1 NewEncoder.Encode).
func MarshalWrite(w io.Writer, v any) error {
	return json.MarshalWrite(w, v, defaultMarshalOpts)
}

// MarshalIndent serializes v to pretty-printed JSON with 2-space indent.
// prefix is accepted for v1 compatibility but is always empty in this repo.
func MarshalIndent(v any, indent string) ([]byte, error) {
	_ = indent // prefix not used; repo standard is indent="  "
	return json.Marshal(v, indentOpts)
}

// MarshalLine serializes v to w followed by a newline.
// Equivalent to v1 json.NewEncoder(w).Encode(v).
func MarshalLine(w io.Writer, v any) error {
	if err := json.MarshalWrite(w, v, defaultMarshalOpts); err != nil {
		return err
	}
	_, err := w.Write([]byte{'\n'})
	return err
}

// Unmarshal deserializes data into v with repo-default options
// (Duration string/number round-trip).
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v, defaultUnmarshalOpts)
}

// UnmarshalRead deserializes a single JSON value from r (consumes entire reader).
// For HTTP bodies, process stdout, etc.
func UnmarshalRead(r io.Reader, v any) error {
	return json.UnmarshalRead(r, v, defaultUnmarshalOpts)
}

// UnmarshalDecode deserializes the next JSON value from a streaming decoder.
// For NDJSON / multi-value streams.
func UnmarshalDecode(dec *jsontext.Decoder, v any) error {
	return json.UnmarshalDecode(dec, v, defaultUnmarshalOpts)
}

// IsValid reports whether data is syntactically valid JSON.
// Replaces json.Valid from v1.
func IsValid(data []byte) bool {
	return jsontext.Value(data).IsValid()
}
