package kms

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// TestEdgeEncryptDecrypt_emptyPlaintext covers the zero-length plaintext
// boundary through the in-process stub client.
func TestEdgeEncryptDecrypt_emptyPlaintext(t *testing.T) {
	t.Parallel()

	c := mustNew(t, Options{KeyID: "test-key-a", DevStub: true})
	ctx := t.Context()

	ct, err := c.Encrypt(ctx, nil)
	if err != nil {
		t.Fatalf("Encrypt(nil) = %v, want nil", err)
	}

	got, err := c.Decrypt(ctx, ct)
	if err != nil {
		t.Fatalf("Decrypt = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Fatalf("Decrypt = %q, want empty", got)
	}
}

// mustSignKey returns a deterministic valid base64 Ed25519 private key.
// Test-only seed; never production material.
func mustSignKey(tb testing.TB) string {
	tb.Helper()
	priv := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	return base64.StdEncoding.EncodeToString(priv)
}

// mustMacKey returns a deterministic valid base64 32-byte MAC key.
func mustMacKey() string {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// mustSignedDriver builds a driver with both MAC and signing keys set.
func mustSignedDriver(tb testing.TB) crypto.Crypto {
	tb.Helper()
	c, err := NewWithClient(Options{KeyID: "sign-key", Key: mustMacKey(), SignKey: mustSignKey(tb)}, StubClient())
	if err != nil {
		tb.Fatalf("NewWithClient = %v, want nil", err)
	}
	return c
}

func TestEdgeStubClient_EmptyKeyID_Fails(t *testing.T) {
	t.Parallel()

	stub := StubClient()
	ctx := t.Context()
	for _, id := range []string{"", "  "} {
		if _, _, err := stub.GenerateDataKey(ctx, id); !errors.Is(err, crypto.ErrInvalidKey) {
			t.Errorf("GenerateDataKey(%q) err = %v, want ErrInvalidKey", id, err)
		}
	}
}

func TestEdgeStubClient_DecryptBadEnvelope_Fails(t *testing.T) {
	t.Parallel()

	stub := StubClient()
	ctx := t.Context()
	cases := map[string][]byte{
		"bad prefix":   []byte("not-a-stub-envelope"),
		"empty":        nil,
		"blank key id": []byte("stub-enc:   "),
	}
	for name, in := range cases {
		if _, err := stub.Decrypt(ctx, in); !errors.Is(err, crypto.ErrIntegrity) {
			t.Errorf("%s: Decrypt err = %v, want ErrIntegrity", name, err)
		}
	}
}

func TestEdgeValidate_Table(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("k", 65536)
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "minimal valid", opts: Options{KeyID: "k1"}},
		{name: "full valid", opts: Options{KeyID: "k1", KeyIDs: []string{"k2"}, Key: mustMacKey(), Region: "us-east-1", Endpoint: "https://kms.example.com", UseEnvelope: true}},
		{name: "empty key id", opts: Options{}, wantErr: true},
		{name: "blank key id", opts: Options{KeyID: "  "}, wantErr: true},
		{name: "overlong key id", opts: Options{KeyID: big}, wantErr: true},
		{name: "blank rotation id", opts: Options{KeyID: "k1", KeyIDs: []string{""}}, wantErr: true},
		{name: "overlong rotation id", opts: Options{KeyID: "k1", KeyIDs: []string{big}}, wantErr: true},
		{name: "key bad base64", opts: Options{KeyID: "k1", Key: "!!!"}, wantErr: true},
		{name: "key wrong length", opts: Options{KeyID: "k1", Key: base64.StdEncoding.EncodeToString(make([]byte, 16))}, wantErr: true},
		{name: "sign key bad base64", opts: Options{KeyID: "k1", SignKey: "!!!"}, wantErr: true},
		{name: "sign key wrong length", opts: Options{KeyID: "k1", SignKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}, wantErr: true},
		{name: "endpoint no scheme", opts: Options{KeyID: "k1", Endpoint: "kms.example.com"}, wantErr: true},
		{name: "endpoint no host", opts: Options{KeyID: "k1", Endpoint: "https:///path"}, wantErr: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.opts.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
			if _, err := NewWithClient(tt.opts, StubClient()); (err != nil) != tt.wantErr {
				t.Errorf("NewWithClient() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestEdgeNewWithClient_NilClient_Fails(t *testing.T) {
	t.Parallel()

	if _, err := NewWithClient(Options{KeyID: "k1"}, nil); !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("NewWithClient(nil) err = %v, want ErrInvalidKey", err)
	}
}

// failClient fails every KMS call with a fixed error.
type failClient struct{ err error }

func (c failClient) GenerateDataKey(context.Context, string) ([]byte, []byte, error) {
	return nil, nil, c.err
}

func (c failClient) Decrypt(context.Context, []byte) ([]byte, error) { return nil, c.err }

// shortKeyClient returns a truncated data key.
type shortKeyClient struct{}

func (shortKeyClient) GenerateDataKey(context.Context, string) ([]byte, []byte, error) {
	return make([]byte, 16), []byte("stub-enc:k"), nil
}

func (shortKeyClient) Decrypt(context.Context, []byte) ([]byte, error) {
	return make([]byte, 16), nil
}

// hugeEncClient returns an overlong encrypted data key.
type hugeEncClient struct{}

func (hugeEncClient) GenerateDataKey(_ context.Context, keyID string) ([]byte, []byte, error) {
	return stubDataKey(keyID), bytes.Repeat([]byte("e"), 65536), nil
}

func (hugeEncClient) Decrypt(ctx context.Context, encrypted []byte) ([]byte, error) {
	return stubClient{}.Decrypt(ctx, encrypted)
}

func TestEdgeEncrypt_KMSFailures_FailClosed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	boom := errors.New("kms boom")

	c, err := NewWithClient(Options{KeyID: "k1"}, failClient{err: boom})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, encErr := c.Encrypt(ctx, []byte("x")); !errors.Is(encErr, boom) {
		t.Errorf("Encrypt KMS error = %v, want kms boom", encErr)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	cc, err := NewWithClient(Options{KeyID: "k1"}, failClient{err: context.Canceled})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, encErr := cc.Encrypt(canceled, []byte("x")); !errors.Is(encErr, context.Canceled) {
		t.Errorf("Encrypt canceled err = %v, want context.Canceled", encErr)
	}

	short, err := NewWithClient(Options{KeyID: "k1"}, shortKeyClient{})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, encErr := short.Encrypt(ctx, []byte("x")); !errors.Is(encErr, crypto.ErrInvalidKey) {
		t.Errorf("Encrypt short DEK err = %v, want ErrInvalidKey", encErr)
	}

	huge, err := NewWithClient(Options{KeyID: "k1"}, hugeEncClient{})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, encErr := huge.Encrypt(ctx, []byte("x")); !errors.Is(encErr, crypto.ErrInvalidKey) {
		t.Errorf("Encrypt huge encDEK err = %v, want ErrInvalidKey", encErr)
	}
}

func TestEdgeEncrypt_OversizeKeyID_FailsWithoutKMSCall(t *testing.T) {
	t.Parallel()

	cc := &countingClient{}
	d := &driver{keyID: strings.Repeat("k", 65536), allowed: map[string]struct{}{}, client: cc}
	if _, err := d.Encrypt(t.Context(), []byte("x")); !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("Encrypt oversize KeyID err = %v, want ErrInvalidKey", err)
	}
	if cc.count() != 0 {
		t.Fatalf("KMS calls = %d, want 0", cc.count())
	}
}

func TestEdgeDecrypt_TruncatedEnvelope_Fails(t *testing.T) {
	t.Parallel()

	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	enc, err := c.Encrypt(ctx, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt = %v, want nil", err)
	}
	// Cut inside the encrypted-DEK region: keyID still parses, DEK does not.
	prefix := 1 + 2 + len("key-a") + 2
	for name, in := range map[string][]byte{
		"cut encDEK": enc[:prefix+3],
		"cut nonce":  enc[:len(enc)-len("secret")-16],
	} {
		if _, err := c.Decrypt(ctx, in); !errors.Is(err, crypto.ErrIntegrity) {
			t.Errorf("%s: Decrypt err = %v, want ErrIntegrity", name, err)
		}
	}
}

func TestEdgeDecrypt_KMSFailures_FailClosed(t *testing.T) {
	t.Parallel()

	stub := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	enc, err := stub.Encrypt(ctx, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt = %v, want nil", err)
	}

	boom := errors.New("decrypt boom")
	bad, err := NewWithClient(Options{KeyID: "key-a"}, failClient{err: boom})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, decErr := bad.Decrypt(ctx, enc); !errors.Is(decErr, crypto.ErrIntegrity) {
		t.Errorf("Decrypt KMS error = %v, want ErrIntegrity", decErr)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	cc, err := NewWithClient(Options{KeyID: "key-a"}, cancelClient{})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, decErr := cc.Decrypt(canceled, enc); !errors.Is(decErr, context.Canceled) {
		t.Errorf("Decrypt canceled err = %v, want context.Canceled", decErr)
	}

	short, err := NewWithClient(Options{KeyID: "key-a"}, shortKeyClient{})
	if err != nil {
		t.Fatalf("NewWithClient = %v, want nil", err)
	}
	if _, err := short.Decrypt(ctx, enc); !errors.Is(err, crypto.ErrIntegrity) {
		t.Errorf("Decrypt short DEK err = %v, want ErrIntegrity", err)
	}
}

func TestEdgeSignVerify_RoundTrip(t *testing.T) {
	t.Parallel()

	c := mustSignedDriver(t)
	ctx := t.Context()
	msg := []byte("sign me")

	sig, err := c.Sign(ctx, msg)
	if err != nil {
		t.Fatalf("Sign = %v, want nil", err)
	}
	if len(sig) == 0 {
		t.Fatal("Sign returned empty signature")
	}

	ok, err := c.Verify(ctx, msg, sig)
	if err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
	if !ok {
		t.Fatal("Verify(valid) = false, want true")
	}

	ok, err = c.Verify(ctx, []byte("other"), sig)
	if err != nil {
		t.Fatalf("Verify(tampered) = %v, want nil", err)
	}
	if ok {
		t.Fatal("Verify(tampered) = true, want false")
	}

	ok, err = c.Verify(ctx, msg, []byte("short"))
	if err != nil {
		t.Fatalf("Verify(short sig) = %v, want nil", err)
	}
	if ok {
		t.Fatal("Verify(short sig) = true, want false")
	}
}

func TestEdgeSignVerify_WithoutKey_Fails(t *testing.T) {
	t.Parallel()

	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	if _, err := c.Sign(ctx, []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("Sign without key err = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.Verify(ctx, []byte("m"), []byte("s")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("Verify without key err = %v, want ErrKeyNotFound", err)
	}
}

func TestEdgeMacVerifyMac_RoundTrip(t *testing.T) {
	t.Parallel()

	c := mustSignedDriver(t)
	ctx := t.Context()
	msg := []byte("mac me")

	mac, err := c.Mac(ctx, msg)
	if err != nil {
		t.Fatalf("Mac = %v, want nil", err)
	}
	if len(mac) != 32 {
		t.Fatalf("Mac len = %d, want 32", len(mac))
	}

	ok, err := c.VerifyMac(ctx, msg, mac)
	if err != nil {
		t.Fatalf("VerifyMac = %v, want nil", err)
	}
	if !ok {
		t.Fatal("VerifyMac(valid) = false, want true")
	}

	bad := bytes.Clone(mac)
	bad[0] ^= 0xff
	if ok, err := c.VerifyMac(ctx, msg, bad); !errors.Is(err, crypto.ErrIntegrity) || ok {
		t.Errorf("VerifyMac(tampered) = (%v, %v), want (false, ErrIntegrity)", ok, err)
	}

	if ok, err := c.VerifyMac(ctx, []byte("other"), mac); !errors.Is(err, crypto.ErrIntegrity) || ok {
		t.Errorf("VerifyMac(other msg) = (%v, %v), want (false, ErrIntegrity)", ok, err)
	}
}

func TestEdgeMacVerifyMac_WithoutKey_Fails(t *testing.T) {
	t.Parallel()

	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	if _, err := c.Mac(ctx, []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("Mac without key err = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.VerifyMac(ctx, []byte("m"), []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("VerifyMac without key err = %v, want ErrKeyNotFound", err)
	}
}

func TestEdgeRegister_WiresFactoryFailClosed(t *testing.T) {
	Register()

	// The registry factory uses New without DevStub, so Open must fail
	// closed rather than silently use the deterministic stub client.
	_, err := crypto.Open(crypto.AdapterKMS, crypto.Options{KeyID: "reg-key"})
	if !errors.Is(err, crypto.ErrInvalidKey) {
		t.Fatalf("Open(kms) err = %v, want ErrInvalidKey", err)
	}
}
