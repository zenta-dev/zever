package local

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/crypto"
)

func b64Bytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		opts    crypto.Options
		wantErr bool
	}{
		{name: "valid key only", opts: crypto.Options{Key: b64Bytes(32)}, wantErr: false},
		{name: "valid key and signKey", opts: crypto.Options{Key: b64Bytes(32), SignKey: b64Bytes(64)}, wantErr: false},
		{name: "empty Key", opts: crypto.Options{}, wantErr: true},
		{name: "bad base64", opts: crypto.Options{Key: "!!! not base64 !!!"}, wantErr: true},
		{name: "wrong len 16", opts: crypto.Options{Key: b64Bytes(16)}, wantErr: true},
		{name: "wrong len 31", opts: crypto.Options{Key: b64Bytes(31)}, wantErr: true},
		{name: "SignKey bad base64", opts: crypto.Options{Key: b64Bytes(32), SignKey: "!!!"}, wantErr: true},
		{name: "SignKey wrong len 63", opts: crypto.Options{Key: b64Bytes(32), SignKey: b64Bytes(63)}, wantErr: true},
		{name: "SignKey wrong len 32", opts: crypto.Options{Key: b64Bytes(32), SignKey: b64Bytes(32)}, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.opts)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr {
				if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
					t.Fatalf("err = %v, want ErrInvalidOptions or ErrInvalidKey", err)
				}
			}
		})
	}
}

func TestNew_EmptyKey_Wrapped(t *testing.T) {
	t.Parallel()
	_, err := New(crypto.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("want ErrInvalidOptions or ErrInvalidKey, got %v", err)
	}
}

func TestNew_BadBase64(t *testing.T) {
	t.Parallel()
	_, err := New(crypto.Options{Key: "!!!"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("want invalid key error, got %v", err)
	}
}

func TestNew_WrongLen16(t *testing.T) {
	t.Parallel()
	_, err := New(crypto.Options{Key: b64Bytes(16)})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("want invalid key error, got %v", err)
	}
}

func TestNew_SignKeyBadBase64(t *testing.T) {
	t.Parallel()
	_, err := New(crypto.Options{Key: b64Bytes(32), SignKey: "!!!"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("want invalid key error, got %v", err)
	}
}

func TestNew_SignKeyWrongLen63(t *testing.T) {
	t.Parallel()
	_, err := New(crypto.Options{Key: b64Bytes(32), SignKey: b64Bytes(63)})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) && !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("want invalid key error, got %v", err)
	}
}
