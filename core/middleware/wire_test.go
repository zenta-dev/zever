package middleware

import (
	"encoding/json"
	"testing"
)

// The tests in this file pin the JSON error envelope shared by Recover,
// RateLimit, Timeout, SecurityHeaders and Cors: the exact key name and
// byte shape clients parse. The envelope is written by errorBodyCodec; a
// stripped or renamed tag changes the client-visible error body while
// still compiling and passing round-trip tests through the same struct.

func TestErrorBodyWireKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   errorBody
		golden string
	}{
		{"recover", errorBody{Error: "internal error"}, `{"error":"internal error"}`},
		{"ratelimit", errorBody{Error: "rate limit exceeded"}, `{"error":"rate limit exceeded"}`},
		{"timeout", errorBody{Error: "request timed out"}, `{"error":"request timed out"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := errorBodyCodec.Encode(tc.body)
			if err != nil {
				t.Fatalf("encode errorBody: %v", err)
			}

			if string(out) != tc.golden {
				t.Errorf("marshal errorBody:\n got %s\nwant %s", out, tc.golden)
			}

			// Decode direction: the golden bytes must load back into the
			// same envelope with the same message.
			got, err := errorBodyCodec.Decode(out)
			if err != nil {
				t.Fatalf("decode golden errorBody: %v", err)
			}

			if got.Error != tc.body.Error {
				t.Errorf("decoded Error = %q, want %q", got.Error, tc.body.Error)
			}
		})
	}
}

func TestErrorBodyOmitsNoKeys(t *testing.T) {
	t.Parallel()

	// The envelope must carry exactly one key; an empty message still
	// serializes the "error" key so clients can rely on its presence.
	out, err := errorBodyCodec.Encode(errorBody{})
	if err != nil {
		t.Fatalf("encode empty errorBody: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal empty errorBody: %v", err)
	}

	if len(m) != 1 {
		t.Fatalf("empty errorBody marshaled %d keys %v, want exactly 1", len(m), m)
	}

	if _, ok := m["error"]; !ok {
		t.Errorf("empty errorBody marshaled %v, want an \"error\" key", m)
	}
}
