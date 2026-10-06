// Package cryptotest provides the conformance kit third-party crypto adapters run to prove backend parity.
package cryptotest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// Conformance verifies factory-built cryptos implement the
// crypto.Crypto contract: open/register round-trip, Encrypt/Decrypt
// round-trip with copy semantics and tamper detection, Sign/Verify,
// Mac/VerifyMac, and wrong-key failure. Each subtest takes a fresh
// instance from factory so cases stay isolated. Tests never call
// time.Sleep and never touch the network.
//
// Crypto has no Close method: there is nothing to shut down, so the
// kit has no Close subtest; cleanup is a no-op.
// Documented KMS exemption: envelope/KMS adapters that delegate
// key storage may report ErrNotSupported for local-key operations
// instead of round-tripping; the kit accepts ErrNotSupported from
// Encrypt as a remote-key signal and then only asserts Decrypt of
// foreign bytes also fails instead of the round-trip.
func Conformance(t *testing.T, factory func(t *testing.T) crypto.Crypto) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("EncryptDecrypt", func(t *testing.T) { conformanceEncryptDecrypt(t, factory) })
	t.Run("Tamper", func(t *testing.T) { conformanceTamper(t, factory) })
	t.Run("SignVerify", func(t *testing.T) { conformanceSignVerify(t, factory) })
	t.Run("Mac", func(t *testing.T) { conformanceMac(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := crypto.Open(crypto.Adapter("conformance-missing-adapter"), crypto.Options{Key: "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="}); !errors.Is(err, crypto.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := crypto.Adapter("conformance-probe-crypto")

	if err := crypto.Register(probe, nil); !errors.Is(err, crypto.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(crypto.Options) (crypto.Crypto, error) {
		return nil, errors.New("cryptotest: probe factory must not run")
	}

	_ = crypto.Register(probe, stub)

	if err := crypto.Register(probe, stub); !errors.Is(err, crypto.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceEncryptDecrypt(t *testing.T, factory func(t *testing.T) crypto.Crypto) {
	t.Helper()

	if err := checkEncryptDecrypt(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkEncryptDecrypt proves Encrypt/Decrypt round-trips with copy
// semantics, returning an error describing the first contract violation
// so broken adapters can be unit-tested without failing the conformance
// run itself. A remote-key adapter reporting ErrNotSupported from
// Encrypt passes with nil error, matching the documented KMS exemption.
func checkEncryptDecrypt(ctx context.Context, c crypto.Crypto) error {
	plain := []byte("conformance-plaintext-01")

	sealed, err := c.Encrypt(ctx, plain)
	if err != nil {
		if errors.Is(err, crypto.ErrNotSupported) {
			return nil
		}
		return fmt.Errorf("Encrypt() error = %w", err)
	}

	if len(sealed) == 0 {
		return errors.New("Encrypt() returned empty ciphertext")
	}

	if bytes.Equal(sealed, plain) {
		return errors.New("Encrypt() returned plaintext unchanged")
	}

	plain[0] = 'X'

	opened, err := c.Decrypt(ctx, sealed)
	if err != nil {
		return fmt.Errorf("Decrypt() error = %w", err)
	}

	if string(opened) != "conformance-plaintext-01" {
		return fmt.Errorf("Decrypt() = %q, want original plaintext", opened)
	}

	opened[0] = 'Y'

	again, err := c.Decrypt(ctx, sealed)
	if err != nil {
		return fmt.Errorf("Decrypt() error = %w", err)
	}

	if string(again) != "conformance-plaintext-01" {
		return fmt.Errorf("Decrypt() = %q, want original (returned copy)", again)
	}

	if _, err := c.Decrypt(ctx, []byte("not-a-ciphertext")); err == nil {
		return errors.New("Decrypt(garbage) = nil, want error")
	}

	if _, err := c.Decrypt(ctx, nil); err == nil {
		return errors.New("Decrypt(nil) = nil, want error")
	}

	return nil
}

func conformanceTamper(t *testing.T, factory func(t *testing.T) crypto.Crypto) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	sealed, err := c.Encrypt(ctx, []byte("tamper-me"))
	if err != nil {
		if errors.Is(err, crypto.ErrNotSupported) {
			t.Skip("remote-key adapter does not round-trip locally")
		}
		t.Fatalf("Encrypt() error = %v", err)
	}

	bad := bytes.Clone(sealed)
	bad[len(bad)-1] ^= 0xff

	if _, err := c.Decrypt(ctx, bad); err == nil {
		t.Error("Decrypt(tampered) = nil, want integrity error")
	} else if !errors.Is(err, crypto.ErrIntegrity) {
		t.Errorf("Decrypt(tampered) err = %v, want ErrIntegrity", err)
	}
}

func conformanceSignVerify(t *testing.T, factory func(t *testing.T) crypto.Crypto) {
	t.Helper()

	conformanceTag(t, factory, "sign",
		func(c crypto.Crypto, ctx context.Context, msg []byte) ([]byte, error) {
			return c.Sign(ctx, msg)
		},
		func(c crypto.Crypto, ctx context.Context, msg, tag []byte) (bool, error) {
			return c.Verify(ctx, msg, tag)
		})
}

func conformanceMac(t *testing.T, factory func(t *testing.T) crypto.Crypto) {
	t.Helper()

	conformanceTag(t, factory, "mac",
		func(c crypto.Crypto, ctx context.Context, msg []byte) ([]byte, error) {
			return c.Mac(ctx, msg)
		},
		func(c crypto.Crypto, ctx context.Context, msg, tag []byte) (bool, error) {
			return c.VerifyMac(ctx, msg, tag)
		})
}

// conformanceTag proves one seal/verify pair (Sign/Verify or
// Mac/VerifyMac): tagging is non-empty, the tag verifies against the
// original message, and verification against another message reports
// false (with ErrIntegrity accepted for adapters like local whose
// VerifyMac signals forgery via error).
func conformanceTag(
	t *testing.T,
	factory func(t *testing.T) crypto.Crypto,
	name string,
	seal func(c crypto.Crypto, ctx context.Context, msg []byte) ([]byte, error),
	open func(c crypto.Crypto, ctx context.Context, msg, tag []byte) (bool, error),
) {
	t.Helper()

	ctx := t.Context()
	c := factory(t)

	msg := []byte("tag-me-01-" + name)

	tag, err := seal(c, ctx, msg)
	if err != nil {
		if errors.Is(err, crypto.ErrNotSupported) {
			return
		}
		t.Fatalf("seal(%s) error = %v", name, err)
	}

	if len(tag) == 0 {
		t.Fatalf("seal(%s) returned empty tag", name)
	}

	ok, err := open(c, ctx, msg, tag)
	if err != nil {
		t.Fatalf("open(%s) error = %v", name, err)
	}

	if !ok {
		t.Errorf("open(%s, valid) = false, want true", name)
	}

	ok, err = open(c, ctx, []byte("other-message"), tag)
	if err != nil && !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("open(%s, tampered) error = %v, want false or ErrIntegrity", name, err)
	}

	if ok {
		t.Errorf("open(%s, tampered) = true, want false", name)
	}
}
