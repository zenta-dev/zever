package crypto_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"fmt"

	"github.com/zenta-dev/zever/core/crypto"
)

var cryptoTestSeq int32 = 2000

func freshCryptoAdapter() crypto.Adapter {
	return crypto.Adapter(fmt.Sprintf("test-%d", atomic.AddInt32(&cryptoTestSeq, 1)))
}

func validOpts() crypto.Options {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return crypto.Options{Key: base64.StdEncoding.EncodeToString(k)}
}

type mockCrypto struct{}

func (m *mockCrypto) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext))
	for i, b := range plaintext {
		out[i] = b + 1
	}
	return out, nil
}

func (m *mockCrypto) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		out[i] = b - 1
	}
	return out, nil
}

func (m *mockCrypto) Sign(_ context.Context, message []byte) ([]byte, error) {
	cp := make([]byte, len(message))
	copy(cp, message)
	return cp, nil
}

func (m *mockCrypto) Verify(_ context.Context, message, signature []byte) (bool, error) {
	if len(message) != len(signature) {
		return false, nil
	}
	for i := range message {
		if message[i] != signature[i] {
			return false, nil
		}
	}
	return true, nil
}

func (m *mockCrypto) Mac(_ context.Context, message []byte) ([]byte, error) {
	h := sha256.Sum256(message)
	out := make([]byte, len(h))
	copy(out, h[:])
	return out, nil
}

func (m *mockCrypto) VerifyMac(_ context.Context, message, mac []byte) (bool, error) {
	h := sha256.Sum256(message)
	if len(mac) != len(h) {
		return false, nil
	}
	for i := range h {
		if mac[i] != h[i] {
			return false, nil
		}
	}
	return true, nil
}

func TestRegister_nil_factory_fails(t *testing.T) {
	t.Parallel()
	a := freshCryptoAdapter()
	err := crypto.Register(a, nil)
	if err == nil {
		t.Fatal("Register nil factory expected error, got nil")
	}
	if !errors.Is(err, crypto.ErrNilFactory) {
		t.Fatalf("err = %v, want ErrNilFactory", err)
	}
	if !strings.Contains(err.Error(), a.String()) {
		t.Fatalf("err = %q, want adapter name", err.Error())
	}
}

func TestRegister_duplicate_fails(t *testing.T) {
	t.Parallel()
	a := freshCryptoAdapter()
	factory := func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }
	if err := crypto.Register(a, factory); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := crypto.Register(a, factory)
	if err == nil {
		t.Fatal("duplicate Register expected error, got nil")
	}
	if !errors.Is(err, crypto.ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
	var de *crypto.DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("err type = %T, want *DuplicateError", err)
	}
	if de.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknown_adapter_fails(t *testing.T) {
	t.Parallel()
	a := freshCryptoAdapter()
	got, err := crypto.Open(a, validOpts())
	if err == nil {
		t.Fatal("Open unknown expected error, got nil")
	}
	if got != nil {
		t.Fatalf("Open unknown got = %v, want nil", got)
	}
	if !errors.Is(err, crypto.ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}
	var ue *crypto.UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err type = %T, want *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", ue.Adapter, a)
	}
}

func TestOpen_validate_before_lookup_fail_closed(t *testing.T) {
	t.Parallel()
	// Unknown adapter but invalid opts should return ErrInvalidOptions, not ErrUnknownAdapter
	a := freshCryptoAdapter()
	_, err := crypto.Open(a, crypto.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, crypto.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions (fail-closed)", err)
	}
	if errors.Is(err, crypto.ErrUnknownAdapter) {
		t.Fatalf("should not be ErrUnknownAdapter when opts invalid")
	}
}

func TestOpen_factory_error_wrapped(t *testing.T) {
	t.Parallel()
	a := freshCryptoAdapter()
	sentinel := errors.New("boom")
	factory := func(crypto.Options) (crypto.Crypto, error) { return nil, sentinel }
	if err := crypto.Register(a, factory); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := crypto.Open(a, validOpts())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Fatalf("got = %v, want nil on factory error", got)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "crypto: open ") {
		t.Fatalf("err = %q, want %q prefix", err.Error(), "crypto: open ")
	}
}

func TestOpen_registered_success(t *testing.T) {
	t.Parallel()
	a := freshCryptoAdapter()
	want := &mockCrypto{}
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return want, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := crypto.Open(a, validOpts())
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if got == nil {
		t.Fatal("Open returned nil")
	}
	// Exercise mockCrypto to cover methods and ensure behavior
	ctx := t.Context()
	enc, err := got.Encrypt(ctx, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	dec, err := got.Decrypt(ctx, enc)
	if err != nil {
		t.Fatalf("Decrypt err = %v", err)
	}
	if dec[0] != 1 || dec[1] != 2 || dec[2] != 3 {
		t.Fatalf("Decrypt roundtrip = %v, want [1 2 3]", dec)
	}
	sig, err := got.Sign(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Sign err = %v", err)
	}
	ok, err := got.Verify(ctx, []byte("hello"), sig)
	if err != nil || !ok {
		t.Fatalf("Verify = %v, err = %v, want true", ok, err)
	}
	mac, err := got.Mac(ctx, []byte("msg"))
	if err != nil {
		t.Fatalf("Mac err = %v", err)
	}
	ok, err = got.VerifyMac(ctx, []byte("msg"), mac)
	if err != nil || !ok {
		t.Fatalf("VerifyMac = %v, err = %v, want true", ok, err)
	}
}

func TestOpen_concurrent_Register_Open(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			a := crypto.Adapter(fmt.Sprintf("test-%d", 100+idx))
			// Register may already be done by parallel runs across other test invocations;
			// use fresh distinct adapters via 100+idx per this test's isolated run.
			// To avoid cross-test collision, we shift by a generation when needed.
			// But Adapter(100+i) is spec-required, so we attempt Register and ignore duplicate.
			_ = crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil })
			_, _ = crypto.Open(a, validOpts())
		}(i)
	}
	wg.Wait()
	// Verify all 8 are openable
	for i := range 8 {
		a := crypto.Adapter(fmt.Sprintf("test-%d", 100+i))
		c, err := crypto.Open(a, validOpts())
		if err != nil {
			t.Fatalf("Open adapter %s err = %v", a, err)
		}
		if c == nil {
			t.Fatalf("Open adapter %s got nil", a)
		}
	}
}

func TestCrypto_mock_increment_and_mac(t *testing.T) {
	t.Parallel()
	m := &mockCrypto{}
	ctx := t.Context()
	in := []byte{0, 255, 10}
	enc, _ := m.Encrypt(ctx, in)
	if enc[0] != 1 || enc[1] != 0 || enc[2] != 11 {
		t.Fatalf("Encrypt increment failed: %v", enc)
	}
	dec, _ := m.Decrypt(ctx, enc)
	for i := range in {
		if dec[i] != in[i] {
			t.Fatalf("Decrypt mismatch at %d", i)
		}
	}
	mac, _ := m.Mac(ctx, []byte("abc"))
	if len(mac) != sha256.Size {
		t.Fatalf("Mac len = %d, want %d", len(mac), sha256.Size)
	}
	ok, _ := m.VerifyMac(ctx, []byte("abc"), mac)
	if !ok {
		t.Fatal("VerifyMac expected true")
	}
	ok, _ = m.VerifyMac(ctx, []byte("abc"), []byte("bad"))
	if ok {
		t.Fatal("VerifyMac expected false for bad mac")
	}
}
