// Package passwordtest provides the conformance kit third-party password adapters run to prove backend parity.
package passwordtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

// Conformance verifies factory-built hashers implement the
// password.Hasher contract: open/register round-trip, Hash/Verify
// round-trip with salt uniqueness, wrong-password rejection,
// NeedsRehash, invalid-hash sentinels, and overlong rejection. Each
// subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network.
//
// Password has no Close method: hashers hold no resources, so the
// kit has no Close subtest; cleanup is a no-op.
// Documented exemption: none; every adapter must verify its own
// hashes and reject malformed ones with ErrInvalidHash.
func Conformance(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("HashVerify", func(t *testing.T) { conformanceHashVerify(t, factory) })
	t.Run("WrongPassword", func(t *testing.T) { conformanceWrongPassword(t, factory) })
	t.Run("NeedsRehash", func(t *testing.T) { conformanceNeedsRehash(t, factory) })
	t.Run("InvalidHash", func(t *testing.T) { conformanceInvalidHash(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := password.Open(password.Adapter("conformance-missing-adapter"), password.Options{Time: password.DefaultTime, Memory: password.DefaultMemory, Threads: password.DefaultThreads, SaltLen: password.DefaultSaltLen, KeyLen: password.DefaultKeyLen}); !errors.Is(err, password.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := password.Adapter("conformance-probe-password")

	if err := password.Register(probe, nil); !errors.Is(err, password.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(password.Options) (password.Hasher, error) {
		return nil, errors.New("passwordtest: probe factory must not run")
	}

	_ = password.Register(probe, stub)

	if err := password.Register(probe, stub); !errors.Is(err, password.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceHashVerify(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	if err := checkHashVerify(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkHashVerify proves Hash/Verify round-trips with unique salts,
// returning an error describing the first contract violation so broken
// adapters can be unit-tested without failing the conformance run itself.
func checkHashVerify(ctx context.Context, h password.Hasher) error {
	first, err := h.Hash(ctx, "conformance-password-01")
	if err != nil {
		return fmt.Errorf("passwordtest: Hash() error = %w", err)
	}

	if first == "" || first == "conformance-password-01" {
		return fmt.Errorf("passwordtest: Hash() = %q, want opaque encoded hash", first)
	}

	second, err := h.Hash(ctx, "conformance-password-01")
	if err != nil {
		return fmt.Errorf("passwordtest: Hash() error = %w", err)
	}

	if first == second {
		return errors.New("passwordtest: Hash() returned identical hashes, want unique salts")
	}

	for _, hash := range []string{first, second} {
		ok, err := h.Verify(ctx, hash, "conformance-password-01")
		if err != nil {
			return fmt.Errorf("passwordtest: Verify() error = %w", err)
		}

		if !ok {
			return errors.New("passwordtest: Verify(correct) = false, want true")
		}
	}

	return nil
}

func conformanceWrongPassword(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	if err := checkWrongPassword(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkWrongPassword proves wrong and empty passwords are rejected with
// (false, nil), returning an error describing the first contract violation.
func checkWrongPassword(ctx context.Context, h password.Hasher) error {
	hash, err := h.Hash(ctx, "right-password")
	if err != nil {
		return fmt.Errorf("passwordtest: Hash() error = %w", err)
	}

	ok, err := h.Verify(ctx, hash, "wrong-password")
	if err != nil {
		return fmt.Errorf("passwordtest: Verify(wrong) error = %w, want (false, nil)", err)
	}

	if ok {
		return errors.New("passwordtest: Verify(wrong) = true, want false")
	}

	ok, err = h.Verify(ctx, hash, "")
	if err != nil {
		return fmt.Errorf("passwordtest: Verify(empty) error = %w, want (false, nil)", err)
	}

	if ok {
		return errors.New("passwordtest: Verify(empty) = true, want false")
	}

	return nil
}

func conformanceNeedsRehash(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	if err := checkNeedsRehash(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkNeedsRehash proves a freshly minted hash does not need rehash,
// returning an error describing the first contract violation.
func checkNeedsRehash(ctx context.Context, h password.Hasher) error {
	hash, err := h.Hash(ctx, "rehash-check")
	if err != nil {
		return fmt.Errorf("passwordtest: Hash() error = %w", err)
	}

	// A hash just minted with current parameters must not need rehash.
	// Adapters without parameter tracking report false with nil error.
	needed, err := h.NeedsRehash(ctx, hash)
	if err != nil {
		return fmt.Errorf("passwordtest: NeedsRehash(current) error = %w", err)
	}

	if needed {
		return errors.New("passwordtest: NeedsRehash(current) = true, want false")
	}

	return nil
}

func conformanceInvalidHash(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	if err := checkInvalidHash(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidHash proves malformed hashes are rejected with
// ErrInvalidHash and overlong passwords with ErrPasswordTooLong,
// returning an error describing the first contract violation.
func checkInvalidHash(ctx context.Context, h password.Hasher) error {
	for _, bad := range []string{"", "not-a-hash", "$argon2id$v=19$m=1,t=1,p=1$c2FsdA$hash"} {
		if _, err := h.Verify(ctx, bad, "whatever"); !errors.Is(err, password.ErrInvalidHash) {
			if err == nil {
				return fmt.Errorf("passwordtest: Verify(%q) = nil, want ErrInvalidHash", bad)
			}
			return fmt.Errorf("passwordtest: Verify(%q) err = %w, want ErrInvalidHash", bad, err)
		}

		if _, err := h.NeedsRehash(ctx, bad); !errors.Is(err, password.ErrInvalidHash) {
			if err == nil {
				return fmt.Errorf("passwordtest: NeedsRehash(%q) = nil, want ErrInvalidHash", bad)
			}
			return fmt.Errorf("passwordtest: NeedsRehash(%q) err = %w, want ErrInvalidHash", bad, err)
		}
	}

	if _, err := h.Hash(ctx, strings.Repeat("p", 1025)); err == nil {
		return errors.New("passwordtest: Hash(overlong) = nil, want ErrPasswordTooLong")
	} else if !errors.Is(err, password.ErrPasswordTooLong) {
		return fmt.Errorf("passwordtest: Hash(overlong) err = %w, want ErrPasswordTooLong", err)
	}

	return nil
}
