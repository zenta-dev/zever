package crypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func TestEdgeOptionsValidate_devKey(t *testing.T) {
	t.Parallel()

	// DevCryptoKey is a valid 32-byte base64 key.
	if err := (crypto.Options{Key: crypto.DevCryptoKey}).Validate(); err != nil {
		t.Fatalf("Validate(DevCryptoKey) err = %v, want nil", err)
	}
}

func TestEdgeOptionsValidate_useEnvelopeRequiresKeyID(t *testing.T) {
	t.Parallel()

	err := (crypto.Options{UseEnvelope: true}).Validate()
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "key_id is required") {
		t.Fatalf("err %q missing key_id reason", err.Error())
	}
}

func TestEdgeOptionsValidate_keyIDsEmptyEntry(t *testing.T) {
	t.Parallel()

	err := (crypto.Options{Key: b64(32), KeyIDs: []string{"k1", ""}}).Validate()
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "key_ids must not contain empty entries") {
		t.Fatalf("err %q missing key_ids reason", err.Error())
	}
}

func TestEdgeOptionsValidate_keyIDsValid(t *testing.T) {
	t.Parallel()

	opts := crypto.Options{Key: b64(32), KeyIDs: []string{"k1", "k2"}}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate err = %v, want nil", err)
	}
}

func TestEdgeOptionsValidate_endpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"valid", "https://kms.example.com", false},
		{"no scheme", "kms.example.com", true},
		{"no host", "https:///path", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (crypto.Options{Key: b64(32), Endpoint: tc.endpoint}).Validate()
			if tc.wantErr && !errors.Is(err, crypto.ErrInvalidOptions) {
				t.Fatalf("Validate(%q) err = %v, want ErrInvalidOptions", tc.endpoint, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) err = %v, want nil", tc.endpoint, err)
			}
		})
	}
}

func TestEdgeIsDevCryptoKey_validNonDevKey(t *testing.T) {
	t.Parallel()

	// A well-formed 32-byte key that is not the dev key must report false.
	if crypto.IsDevCryptoKey(b64(32)) {
		t.Fatal("IsDevCryptoKey(valid non-dev key) = true, want false")
	}
}

func TestEdgeInvalidOptionsError_emptyReason(t *testing.T) {
	t.Parallel()

	err := crypto.InvalidOptionsError{}
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.HasSuffix(err.Error(), ": ") {
		t.Fatalf("Error() = %q, want trailing sentinel with empty reason", err.Error())
	}
}
