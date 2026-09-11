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

	if !strings.HasPrefix(err.Error(), "codec: encode failed: ") {
		t.Errorf("err.Error() = %q, want prefix %q", err.Error(), "codec: encode failed: ")
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
