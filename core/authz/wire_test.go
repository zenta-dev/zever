package authz

import (
	"testing"
)

// The tests in this file pin the JSON error envelope written by
// writeAuthzError: the exact key name clients parse. A stripped or renamed
// tag changes the client-visible error body while still compiling and
// passing round-trip tests through the same struct.

func TestAuthzErrorBodyWireKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   authzErrorBody
		golden string
	}{
		{"denied", authzErrorBody{Error: "permission denied"}, `{"error":"permission denied"}`},
		{"unauthorized", authzErrorBody{Error: "unauthorized"}, `{"error":"unauthorized"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := authzErrorCodec.Encode(tc.body)
			if err != nil {
				t.Fatalf("encode authzErrorBody: %v", err)
			}

			if string(out) != tc.golden {
				t.Errorf("marshal authzErrorBody:\n got %s\nwant %s", out, tc.golden)
			}

			got, err := authzErrorCodec.Decode(out)
			if err != nil {
				t.Fatalf("decode golden authzErrorBody: %v", err)
			}

			if got.Error != tc.body.Error {
				t.Errorf("decoded Error = %q, want %q", got.Error, tc.body.Error)
			}
		})
	}
}

func TestAuthzErrorBodyOmitsNoKeys(t *testing.T) {
	t.Parallel()

	// The envelope must carry exactly one key; an empty message still
	// serializes the "error" key so clients can rely on its presence.
	out, err := authzErrorCodec.Encode(authzErrorBody{})
	if err != nil {
		t.Fatalf("encode empty authzErrorBody: %v", err)
	}

	if string(out) != `{"error":""}` {
		t.Errorf("empty authzErrorBody marshaled %s, want %q", out, `{"error":""}`)
	}
}
