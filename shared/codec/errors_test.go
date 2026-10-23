package codec

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestErrSentinelMessages(t *testing.T) {
	t.Parallel()

	if got := ErrEncode.Error(); got != "codec: encode failed" {
		t.Errorf("ErrEncode.Error() = %q, want %q", got, "codec: encode failed")
	}

	if got := ErrDecode.Error(); got != "codec: decode failed" {
		t.Errorf("ErrDecode.Error() = %q, want %q", got, "codec: decode failed")
	}
}

func TestJSONCodecEncodeWrapsSentinel(t *testing.T) {
	t.Parallel()

	_, err := JSONCodec[float64]{}.Encode(math.NaN())
	if err == nil {
		t.Fatal("Encode() error = nil, want wrapped ErrEncode")
	}

	if !errors.Is(err, ErrEncode) {
		t.Errorf("errors.Is(err, ErrEncode) = false, want true (err = %v)", err)
	}

	var semErr *json.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error type = %T, want *json.SemanticError (err = %v)", err, err)
	}

	if !strings.HasPrefix(err.Error(), "codec: encode float64 failed: ") {
		t.Errorf("err.Error() = %q, want prefix %q", err.Error(), "codec: encode float64 failed: ")
	}

	// The message stays on one line: a multi-line error loses its cause to
	// every single-line consumer (the CLI doctor note truncates at the
	// first newline).
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("err.Error() = %q, want a single-line message", err.Error())
	}
}

func TestJSONCodecDecodeWrapsSentinel(t *testing.T) {
	t.Parallel()

	_, err := JSONCodec[int]{}.Decode([]byte(`{invalid`))
	if err == nil {
		t.Fatal("Decode() error = nil, want wrapped ErrDecode")
	}

	if !errors.Is(err, ErrDecode) {
		t.Errorf("errors.Is(err, ErrDecode) = false, want true (err = %v)", err)
	}

	var synErr *jsontext.SyntacticError
	if !errors.As(err, &synErr) {
		t.Fatalf("error type = %T, want *jsontext.SyntacticError (err = %v)", err, err)
	}
}

// TestJSONCodecDecodeNamesTheTargetType proves the decode failure says which
// type the bytes were being decoded into.
func TestJSONCodecDecodeNamesTheTargetType(t *testing.T) {
	t.Parallel()

	_, err := JSONCodec[int]{}.Decode([]byte(`{invalid`))
	if err == nil {
		t.Fatal("Decode() error = nil, want wrapped ErrDecode")
	}

	if !strings.HasPrefix(err.Error(), "codec: decode into int failed: ") {
		t.Errorf("err.Error() = %q, want prefix %q", err.Error(), "codec: decode into int failed: ")
	}

	if strings.Contains(err.Error(), "\n") {
		t.Errorf("err.Error() = %q, want a single-line message", err.Error())
	}
}

// TestJSONCodecDecodeInterfaceType proves a nil interface value never
// renders as "<nil>": the codec's declared type is named instead.
func TestJSONCodecDecodeInterfaceType(t *testing.T) {
	t.Parallel()

	_, err := JSONCodec[any]{}.Decode([]byte(`{invalid`))
	if err == nil {
		t.Fatal("Decode() error = nil, want wrapped ErrDecode")
	}

	if !strings.HasPrefix(err.Error(), "codec: decode into interface {} failed: ") {
		t.Errorf("err.Error() = %q, want the declared type named", err.Error())
	}
}
