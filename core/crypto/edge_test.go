package crypto_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func TestEdgeOptions_Validate_table(t *testing.T) {
	t.Parallel()

	key32 := base64.StdEncoding.EncodeToString(make([]byte, 32))
	key64 := base64.StdEncoding.EncodeToString(make([]byte, 64))
	key16 := base64.StdEncoding.EncodeToString(make([]byte, 16))

	tests := []struct {
		name    string
		opts    crypto.Options
		wantErr bool
	}{
		{"valid key", crypto.Options{Key: key32}, false},
		{"empty key", crypto.Options{}, true},
		{"bad base64", crypto.Options{Key: "!!!"}, true},
		{"wrong key length", crypto.Options{Key: key16}, true},
		{"valid sign key", crypto.Options{Key: key32, SignKey: key64}, false},
		{"wrong sign key length", crypto.Options{Key: key32, SignKey: key16}, true},
		{"kms only no key", crypto.Options{KeyID: "kms-1"}, false},
		{"envelope without key id", crypto.Options{UseEnvelope: true}, true},
		{"envelope with key id", crypto.Options{UseEnvelope: true, KeyID: "kms-1"}, false},
		{"key ids empty entry", crypto.Options{Key: key32, KeyIDs: []string{"kms-1", " "}}, true},
		{"endpoint no scheme", crypto.Options{Key: key32, Endpoint: "kms.example.com"}, true},
		{"endpoint no host", crypto.Options{Key: key32, Endpoint: "https://"}, true},
		{"endpoint valid", crypto.Options{Key: key32, Endpoint: "https://kms.example.com"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if tc.wantErr && !errors.Is(err, crypto.ErrInvalidOptions) {
				t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}
