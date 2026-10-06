package cryptotest

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// stubCrypto is a scriptable crypto.Crypto for proving checkEncryptDecrypt
// catches broken adapters. Unscripted operations report ErrNotSupported.
type stubCrypto struct {
	onEncrypt func(ctx context.Context, plain []byte) ([]byte, error)
	onDecrypt func(ctx context.Context, sealed []byte) ([]byte, error)
}

func (s stubCrypto) Encrypt(ctx context.Context, plain []byte) ([]byte, error) {
	if s.onEncrypt != nil {
		return s.onEncrypt(ctx, plain)
	}
	return nil, crypto.ErrNotSupported
}

func (s stubCrypto) Decrypt(ctx context.Context, sealed []byte) ([]byte, error) {
	if s.onDecrypt != nil {
		return s.onDecrypt(ctx, sealed)
	}
	return nil, crypto.ErrIntegrity
}

func (s stubCrypto) Sign(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrNotSupported
}

func (s stubCrypto) Verify(context.Context, []byte, []byte) (bool, error) {
	return false, crypto.ErrNotSupported
}

func (s stubCrypto) Mac(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrNotSupported
}

func (s stubCrypto) VerifyMac(context.Context, []byte, []byte) (bool, error) {
	return false, crypto.ErrNotSupported
}

var _ crypto.Crypto = stubCrypto{}

func TestCheckEncryptDecrypt_Negatives(t *testing.T) {
	t.Parallel()

	boom := errors.New("backend boom")
	sealed := []byte("sealed-bytes")
	plain := []byte("conformance-plaintext-01")

	secondBoom := 0
	shared := bytes.Clone(plain)

	tests := []struct {
		name string
		stub stubCrypto
		want string
	}{
		{
			name: "encrypt error",
			stub: stubCrypto{onEncrypt: func(context.Context, []byte) ([]byte, error) { return nil, boom }},
			want: "Encrypt() error",
		},
		{
			name: "empty sealed",
			stub: stubCrypto{onEncrypt: func(context.Context, []byte) ([]byte, error) { return []byte{}, nil }},
			want: "empty ciphertext",
		},
		{
			name: "plaintext echo",
			stub: stubCrypto{onEncrypt: func(_ context.Context, p []byte) ([]byte, error) { return p, nil }},
			want: "plaintext unchanged",
		},
		{
			name: "decrypt error",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(context.Context, []byte) ([]byte, error) { return nil, boom },
			},
			want: "Decrypt() error",
		},
		{
			name: "decrypt mismatch",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(context.Context, []byte) ([]byte, error) { return []byte("wrong"), nil },
			},
			want: "want original plaintext",
		},
		{
			name: "second decrypt error",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(context.Context, []byte) ([]byte, error) {
					secondBoom++
					if secondBoom == 2 {
						return nil, boom
					}
					return bytes.Clone(plain), nil
				},
			},
			want: "Decrypt() error",
		},
		{
			name: "shared buffer",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(context.Context, []byte) ([]byte, error) { return shared, nil },
			},
			want: "want original (returned copy)",
		},
		{
			name: "garbage accepted",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(context.Context, []byte) ([]byte, error) { return bytes.Clone(plain), nil },
			},
			want: "Decrypt(garbage) = nil",
		},
		{
			name: "nil accepted",
			stub: stubCrypto{
				onEncrypt: func(context.Context, []byte) ([]byte, error) { return sealed, nil },
				onDecrypt: func(_ context.Context, s []byte) ([]byte, error) {
					if s == nil {
						return bytes.Clone(plain), nil
					}
					if string(s) == "not-a-ciphertext" {
						return nil, crypto.ErrIntegrity
					}
					return bytes.Clone(plain), nil
				},
			},
			want: "Decrypt(nil) = nil",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := checkEncryptDecrypt(t.Context(), tt.stub); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("checkEncryptDecrypt() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckEncryptDecrypt_RemoteExempt(t *testing.T) {
	t.Parallel()

	if err := checkEncryptDecrypt(t.Context(), stubCrypto{}); err != nil {
		t.Errorf("checkEncryptDecrypt(remote) = %v, want nil", err)
	}
}
