package crypto_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/crypto"
)

func b64(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestOptions_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		opts    crypto.Options
		wantErr bool
		check   func(t *testing.T, err error)
	}{
		{
			name:    "zero fails",
			opts:    crypto.Options{},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, crypto.ErrInvalidOptions) {
					t.Fatalf("err = %v, want ErrInvalidOptions", err)
				}
				var ioe *crypto.InvalidOptionsError
				if !errors.As(err, &ioe) {
					t.Fatalf("err type = %T, want *InvalidOptionsError", err)
				}
				if !strings.Contains(err.Error(), "key is required") {
					t.Fatalf("err = %q, want %q", err.Error(), "key is required")
				}
			},
		},
		{
			name:    "valid 32-byte key passes",
			opts:    crypto.Options{Key: b64(32)},
			wantErr: false,
		},
		{
			name:    "valid 32 and 64 passes",
			opts:    crypto.Options{Key: b64(32), SignKey: b64(64)},
			wantErr: false,
		},
		{
			name:    "bad base64 key",
			opts:    crypto.Options{Key: "!!! not base64 !!!"},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "key must be valid base64") {
					t.Fatalf("err = %q, want %q", err.Error(), "key must be valid base64")
				}
				if !errors.Is(err, crypto.ErrInvalidOptions) {
					t.Fatalf("want ErrInvalidOptions")
				}
			},
		},
		{
			name:    "key wrong length 16",
			opts:    crypto.Options{Key: b64(16)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "key must decode to 32 bytes") {
					t.Fatalf("err = %q, want decode 32", err.Error())
				}
			},
		},
		{
			name:    "key wrong length 31",
			opts:    crypto.Options{Key: b64(31)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "key must decode to 32 bytes") {
					t.Fatalf("err = %q, want decode 32", err.Error())
				}
			},
		},
		{
			name:    "key wrong length 33",
			opts:    crypto.Options{Key: b64(33)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "key must decode to 32 bytes") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
		{
			name:    "signkey optional empty passes",
			opts:    crypto.Options{Key: b64(32), SignKey: ""},
			wantErr: false,
		},
		{
			name:    "signkey bad base64",
			opts:    crypto.Options{Key: b64(32), SignKey: "!!!"},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "sign_key must be valid base64") {
					t.Fatalf("err = %q, want sign_key valid base64", err.Error())
				}
			},
		},
		{
			name:    "signkey wrong len 63",
			opts:    crypto.Options{Key: b64(32), SignKey: b64(63)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "sign_key must decode to 64 bytes") {
					t.Fatalf("err = %q, want 64 bytes", err.Error())
				}
			},
		},
		{
			name:    "signkey wrong len 32",
			opts:    crypto.Options{Key: b64(32), SignKey: b64(32)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "sign_key must decode to 64 bytes") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
		{
			name:    "signkey wrong len 65",
			opts:    crypto.Options{Key: b64(32), SignKey: b64(65)},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !strings.Contains(err.Error(), "sign_key must decode to 64 bytes") {
					t.Fatalf("err = %q", err.Error())
				}
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.opts.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if tc.wantErr && tc.check != nil {
				tc.check(t, err)
			}
		})
	}
}

func TestOptions_Validate_multiple_errors_Join(t *testing.T) {
	t.Parallel()
	// Both Key and SignKey bad → both reasons present, errors.Is still true for ErrInvalidOptions
	opts := crypto.Options{Key: "!!!", SignKey: "!!!"}
	err := opts.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "key must be valid base64") {
		t.Fatalf("err = %q, want key valid base64", err.Error())
	}
	if !strings.Contains(err.Error(), "sign_key must be valid base64") {
		t.Fatalf("err = %q, want sign_key valid base64", err.Error())
	}
	// Also test key required + sign_key bad
	opts2 := crypto.Options{Key: "", SignKey: b64(32)}
	err2 := opts2.Validate()
	if err2 == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err2.Error(), "key is required") {
		t.Fatalf("err2 = %q, want key is required", err2.Error())
	}
	if !strings.Contains(err2.Error(), "sign_key must decode to 64 bytes") {
		t.Fatalf("err2 = %q, want sign_key 64", err2.Error())
	}
	if !errors.Is(err2, crypto.ErrInvalidOptions) {
		t.Fatalf("err2 Is ErrInvalidOptions false")
	}
}

func TestOptions_Validate_both_lengths_wrong_Join(t *testing.T) {
	t.Parallel()
	opts := crypto.Options{Key: b64(16), SignKey: b64(63)}
	err := opts.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "key must decode to 32 bytes") {
		t.Fatalf("err = %q, want key 32", err.Error())
	}
	if !strings.Contains(err.Error(), "sign_key must decode to 64 bytes") {
		t.Fatalf("err = %q, want sign_key 64", err.Error())
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("Is ErrInvalidOptions false")
	}
}
