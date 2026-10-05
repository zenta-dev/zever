package crypto_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// nilCtx is a nil context used to prove the battery never dereferences ctx
// before the adapter boundary.
var nilCtx context.Context

func TestEdgeOpen_emptyAdapterName(t *testing.T) {
	t.Parallel()

	got, err := crypto.Open(crypto.Adapter(""), validOpts())
	if !errors.Is(err, crypto.ErrUnknownAdapter) {
		t.Fatalf("Open(\"\") err = %v, want ErrUnknownAdapter", err)
	}
	var ue crypto.UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != crypto.Adapter("") {
		t.Fatalf("Adapter = %v, want empty", ue.Adapter)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestEdgeOpen_kmsFieldsMakeKeyOptional(t *testing.T) {
	t.Parallel()

	a := freshCryptoAdapter()
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	// Any KMS field set relaxes the required Key check.
	cases := []crypto.Options{
		{KeyID: "k1"},
		{KeyIDs: []string{"k1"}},
		{Region: "us-east-1"},
		{Endpoint: "https://kms.example.com"},
		{UseEnvelope: true, KeyID: "k1"},
	}
	for _, opts := range cases {
		if _, err := crypto.Open(a, opts); err != nil {
			t.Errorf("Open(%+v) err = %v, want nil", opts, err)
		}
	}
}

func TestEdgeOpen_factoryReturnsNilNil(t *testing.T) {
	t.Parallel()

	a := freshCryptoAdapter()
	var nilCrypto crypto.Crypto
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return nilCrypto, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := crypto.Open(a, validOpts())
	if err != nil {
		t.Fatalf("Open err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

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

func TestEdgeEncryptSignMac_nilContext(t *testing.T) {
	t.Parallel()

	a := freshCryptoAdapter()
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := crypto.Open(a, validOpts())
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	enc, err := got.Encrypt(nilCtx, []byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt(nil ctx) err = %v, want nil", err)
	}
	dec, err := got.Decrypt(nilCtx, enc)
	if err != nil {
		t.Fatalf("Decrypt(nil ctx) err = %v, want nil", err)
	}
	if string(dec) != "hello" {
		t.Fatalf("Decrypt roundtrip = %q, want hello", dec)
	}
	sig, err := got.Sign(nilCtx, []byte("msg"))
	if err != nil {
		t.Fatalf("Sign(nil ctx) err = %v, want nil", err)
	}
	ok, err := got.Verify(nilCtx, []byte("msg"), sig)
	if err != nil || !ok {
		t.Fatalf("Verify(nil ctx) = %v, %v, want true, nil", ok, err)
	}
	mac, err := got.Mac(nilCtx, []byte("msg"))
	if err != nil {
		t.Fatalf("Mac(nil ctx) err = %v, want nil", err)
	}
	ok, err = got.VerifyMac(nilCtx, []byte("msg"), mac)
	if err != nil || !ok {
		t.Fatalf("VerifyMac(nil ctx) = %v, %v, want true, nil", ok, err)
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

func TestEdgeParseAdapter_whitespace(t *testing.T) {
	t.Parallel()

	got, err := crypto.ParseAdapter(" ")
	if err != nil {
		t.Fatalf("ParseAdapter(\" \") err = %v, want nil", err)
	}
	if got != crypto.Adapter(" ") {
		t.Fatalf("ParseAdapter(\" \") = %v, want space", got)
	}
}
