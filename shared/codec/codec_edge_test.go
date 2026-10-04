package codec

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestJSONCodecEncode_deterministicMapOrder verifies Encode is byte-for-byte
// stable across repeated calls and map insertion orders, so callers can hash
// or sign the output.
func TestJSONCodecEncode_deterministicMapOrder(t *testing.T) {
	t.Parallel()

	c := JSONCodec[map[string]int]{}

	first := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8}

	second := make(map[string]int, len(first))
	for _, k := range []string{"h", "g", "f", "e", "d", "c", "b", "a"} {
		second[k] = first[k]
	}

	want, err := c.Encode(first)
	if err != nil {
		t.Fatalf("Encode(first) error = %v", err)
	}

	for range 20 {
		got, encErr := c.Encode(first)
		if encErr != nil {
			t.Fatalf("Encode(first) error = %v", encErr)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf("Encode() not deterministic: got %s, want %s", got, want)
		}
	}

	got, err := c.Encode(second)
	if err != nil {
		t.Fatalf("Encode(second) error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("Encode() depends on insertion order: got %s, want %s", got, want)
	}
}

// TestJSONCodec_largePayloadRoundTrip verifies a 1 MiB string survives the
// round trip unmodified.
func TestJSONCodec_largePayloadRoundTrip(t *testing.T) {
	t.Parallel()

	c := JSONCodec[string]{}
	in := strings.Repeat("zever", 1<<18)

	encoded, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if len(encoded) < len(in) {
		t.Fatalf("Encode() len = %d, want >= %d", len(encoded), len(in))
	}

	decoded, err := c.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decoded != in {
		t.Fatal("Decode() did not round-trip the large payload")
	}
}

// TestJSONCodecDecode_trailingDataRejected verifies a valid document followed
// by garbage is rejected instead of silently accepted.
func TestJSONCodecDecode_trailingDataRejected(t *testing.T) {
	t.Parallel()

	c := JSONCodec[map[string]int]{}

	_, err := c.Decode([]byte(`{"a":1} trailing`))
	if err == nil {
		t.Fatal("Decode() = nil error, want trailing-data rejection")
	}

	if !errors.Is(err, ErrDecode) {
		t.Errorf("errors.Is(err, ErrDecode) = false (err = %v)", err)
	}
}
