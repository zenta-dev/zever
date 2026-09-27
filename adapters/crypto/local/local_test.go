package local

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func genAESKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func genSignKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(priv)
}

func mustNew(t *testing.T, opts crypto.Options) crypto.Crypto {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return c
}

func TestNew_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts crypto.Options
	}{
		{name: "valid key only", opts: crypto.Options{Key: genAESKey(t)}},
		{name: "valid key and signKey", opts: crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := New(tc.opts)
			if err != nil {
				t.Fatalf("New err = %v", err)
			}
			if c == nil {
				t.Fatal("expected non-nil crypto")
			}
		})
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	lengths := []int{0, 1, 16, 1024}
	for _, n := range lengths {
		n := n
		t.Run("", func(t *testing.T) {
			t.Parallel()
			plaintext := make([]byte, n)
			if _, err := rand.Read(plaintext); err != nil {
				t.Fatal(err)
			}
			enc, err := c.Encrypt(ctx, plaintext)
			if err != nil {
				t.Fatalf("Encrypt err = %v", err)
			}
			dec, err := c.Decrypt(ctx, enc)
			if err != nil {
				t.Fatalf("Decrypt err = %v", err)
			}
			if !bytes.Equal(dec, plaintext) {
				t.Fatalf("roundtrip mismatch: got %x want %x", dec, plaintext)
			}
			// Ensure ciphertext differs and has nonce prepended
			if n > 0 && bytes.Equal(enc, plaintext) {
				t.Fatal("ciphertext should not equal plaintext")
			}
		})
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	enc, err := c.Encrypt(ctx, []byte("tamper me"))
	if err != nil {
		t.Fatal(err)
	}
	enc[len(enc)-1] ^= 0xff
	_, err = c.Decrypt(ctx, enc)
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity", err)
	}
}

func TestDecrypt_Short(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	_, err := c.Decrypt(ctx, []byte("short"))
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity for short ciphertext", err)
	}
	_, err = c.Decrypt(ctx, []byte{})
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity for empty", err)
	}
	// exactly nonce size -1
	_, err = c.Decrypt(ctx, make([]byte, 11))
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity for <nonce", err)
	}
}

func TestMac_VerifyMac_RoundTrip(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	msg := []byte("mac this")
	mac, err := c.Mac(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.VerifyMac(ctx, msg, mac)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("VerifyMac returned false")
	}
}

func TestMac_EmptyMessage(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	mac, err := c.Mac(ctx, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(mac) == 0 {
		t.Fatal("empty mac")
	}
	ok, err := c.VerifyMac(ctx, []byte{}, mac)
	if err != nil || !ok {
		t.Fatalf("VerifyMac empty = %v err %v", ok, err)
	}
}

func TestVerifyMac_Tampered(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	msg := []byte("mac this")
	mac, err := c.Mac(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	mac[0] ^= 0xff
	ok, err := c.VerifyMac(ctx, msg, mac)
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity", err)
	}
	if ok {
		t.Fatal("expected false for tampered mac")
	}
}

func TestMac_Deterministic_And_Separate(t *testing.T) {
	t.Parallel()
	key := genAESKey(t)
	c, err := New(crypto.Options{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	lc, ok := c.(*localCrypto)
	if !ok {
		t.Fatal("expected *localCrypto")
	}
	if len(lc.macKey) != len(lc.aesKey) {
		t.Fatalf("mac key len %d != aes key len %d", len(lc.macKey), len(lc.aesKey))
	}
	if bytes.Equal(lc.aesKey, lc.macKey) {
		t.Fatal("mac key must not equal aes key")
	}
	ctx := t.Context()
	msg := []byte("determinism")
	mac1, _ := lc.Mac(ctx, msg)
	c2, err := New(crypto.Options{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	lc2, ok := c2.(*localCrypto)
	if !ok {
		t.Fatal("expected *localCrypto")
	}
	mac2, _ := lc2.Mac(ctx, msg)
	if !bytes.Equal(mac1, mac2) {
		t.Fatal("mac must be deterministic across instances")
	}
	// Different key should give different mac
	c3, _ := New(crypto.Options{Key: genAESKey(t)})
	mac3, _ := c3.Mac(ctx, msg)
	if bytes.Equal(mac1, mac3) {
		t.Fatal("different keys should give different macs")
	}
}

func TestSignVerify_WithKey(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)})
	ctx := t.Context()
	msg := []byte("sign me")
	sig, err := c.Sign(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 64 {
		t.Fatalf("sig len = %d, want 64", len(sig))
	}
	ok, err := c.Verify(ctx, msg, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("verify returned false")
	}
}

func TestSignVerify_TamperedSig(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)})
	ctx := t.Context()
	msg := []byte("sign me")
	sig, _ := c.Sign(ctx, msg)
	sig[0] ^= 0xff
	ok, err := c.Verify(ctx, msg, sig)
	if err != nil {
		t.Fatalf("Verify err = %v, want nil", err)
	}
	if ok {
		t.Fatal("expected false for tampered sig")
	}
}

func TestSignVerify_WrongMessage(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)})
	ctx := t.Context()
	msg := []byte("original")
	sig, _ := c.Sign(ctx, msg)
	ok, err := c.Verify(ctx, []byte("different"), sig)
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if ok {
		t.Fatal("expected false for wrong message")
	}
}

func TestSignVerify_NoKey(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	_, err := c.Sign(ctx, []byte("msg"))
	if !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Sign got err %v, want ErrKeyNotFound", err)
	}
	_, err = c.Verify(ctx, []byte("msg"), make([]byte, 64))
	if !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Verify got err %v, want ErrKeyNotFound", err)
	}
}

func TestMac_VerifyMac_NoSignKeyStillWorks(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	mac, err := c.Mac(ctx, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.VerifyMac(ctx, []byte("hello"), mac)
	if !ok || err != nil {
		t.Fatalf("VerifyMac without signKey failed: ok %v err %v", ok, err)
	}
}

func TestNew_Idempotence(t *testing.T) {
	t.Parallel()
	opts := crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)}
	c1, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	msg := []byte("idempotent")
	m1, _ := c1.Mac(ctx, msg)
	m2, _ := c2.Mac(ctx, msg)
	if !bytes.Equal(m1, m2) {
		t.Fatal("idempotent New should give same mac")
	}
	// Encrypt/Decrypt still works across instances
	enc, _ := c1.Encrypt(ctx, msg)
	dec, err := c2.Decrypt(ctx, enc)
	if err != nil || !bytes.Equal(dec, msg) {
		t.Fatalf("cross-instance decrypt failed: %v %x", err, dec)
	}
}

func TestConcurrent_MixedOps(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)})
	ctx := t.Context()
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			msg := []byte("concurrent")
			// Encrypt/Decrypt
			enc, err := c.Encrypt(ctx, msg)
			if err != nil {
				errCh <- err
				return
			}
			dec, err := c.Decrypt(ctx, enc)
			if err != nil {
				errCh <- err
				return
			}
			if !bytes.Equal(dec, msg) {
				errCh <- errors.New("decrypt mismatch")
				return
			}
			// Mac
			mac, err := c.Mac(ctx, msg)
			if err != nil {
				errCh <- err
				return
			}
			ok, err := c.VerifyMac(ctx, msg, mac)
			if err != nil || !ok {
				errCh <- errors.New("mac verify failed")
				return
			}
			// Sign/Verify
			sig, err := c.Sign(ctx, msg)
			if err != nil {
				errCh <- err
				return
			}
			ok, err = c.Verify(ctx, msg, sig)
			if err != nil || !ok {
				errCh <- errors.New("sign verify failed")
				return
			}
			errCh <- nil
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent op failed: %v", err)
		}
	}
}

func TestEncrypt_NonceRandomness(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	msg := []byte("same message")
	e1, _ := c.Encrypt(ctx, msg)
	e2, _ := c.Encrypt(ctx, msg)
	if bytes.Equal(e1, e2) {
		t.Fatal("encrypt should produce different ciphertexts due to random nonce")
	}
}

func TestDecrypt_WrongKey_Fails(t *testing.T) {
	t.Parallel()
	c1 := mustNew(t, crypto.Options{Key: genAESKey(t)})
	c2 := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	enc, _ := c1.Encrypt(ctx, []byte("secret"))
	_, err := c2.Decrypt(ctx, enc)
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("wrong key decrypt got %v, want ErrIntegrity", err)
	}
}

// Registry integration - not parallel due to global map.
func TestRegistry_Integration(t *testing.T) {
	// No t.Parallel() - registry is global
	// Use a fresh adapter to avoid collision with existing registrations
	adapter := crypto.Adapter("test-9999")
	// Ensure not already registered - try register, ignore duplicate from prior run
	_ = crypto.Register(adapter, New)
	// Open via crypto.Open should succeed with valid opts
	validKey := genAESKey(t)
	c, err := crypto.Open(adapter, crypto.Options{Key: validKey})
	if err != nil {
		t.Fatalf("Open via registry err = %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil crypto from registry")
	}
	ctx := t.Context()
	enc, err := c.Encrypt(ctx, []byte("registry"))
	if err != nil {
		t.Fatal(err)
	}
	dec, err := c.Decrypt(ctx, enc)
	if err != nil || string(dec) != "registry" {
		t.Fatalf("registry roundtrip failed: %v %s", err, dec)
	}
}

func TestVerifyMac_WrongMessage(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	mac, _ := c.Mac(ctx, []byte("original"))
	ok, err := c.VerifyMac(ctx, []byte("different"), mac)
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("want ErrIntegrity, got %v", err)
	}
	if ok {
		t.Fatal("expected false for wrong message")
	}
}

func TestEncrypt_RandFailure(t *testing.T) {
	// not parallel: mutates global randRead seam
	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()
	orig := randRead
	randRead = func(_ []byte) (int, error) { return 0, errors.New("rand fail") }
	defer func() { randRead = orig }()
	_, err := c.Encrypt(ctx, []byte("fail"))
	if err == nil {
		t.Fatal("expected error for rand failure")
	}
	if err.Error() != "rand fail" {
		t.Fatalf("got %v, want rand fail", err)
	}
}
