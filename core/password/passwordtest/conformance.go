// Package passwordtest provides the conformance kit third-party password adapters run to prove backend parity.
package passwordtest

import (
	"errors"
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

	ctx := t.Context()
	h := factory(t)

	first, err := h.Hash(ctx, "conformance-password-01")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if first == "" || first == "conformance-password-01" {
		t.Fatalf("Hash() = %q, want opaque encoded hash", first)
	}

	second, err := h.Hash(ctx, "conformance-password-01")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if first == second {
		t.Error("Hash() returned identical hashes, want unique salts")
	}

	for _, hash := range []string{first, second} {
		ok, err := h.Verify(ctx, hash, "conformance-password-01")
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}

		if !ok {
			t.Error("Verify(correct) = false, want true")
		}
	}
}

func conformanceWrongPassword(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	ctx := t.Context()
	h := factory(t)

	hash, err := h.Hash(ctx, "right-password")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	ok, err := h.Verify(ctx, hash, "wrong-password")
	if err != nil {
		t.Fatalf("Verify(wrong) error = %v, want (false, nil)", err)
	}

	if ok {
		t.Error("Verify(wrong) = true, want false")
	}

	ok, err = h.Verify(ctx, hash, "")
	if err != nil {
		t.Fatalf("Verify(empty) error = %v, want (false, nil)", err)
	}

	if ok {
		t.Error("Verify(empty) = true, want false")
	}
}

func conformanceNeedsRehash(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	ctx := t.Context()
	h := factory(t)

	hash, err := h.Hash(ctx, "rehash-check")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	// A hash just minted with current parameters must not need rehash.
	// Adapters without parameter tracking report false with nil error.
	needed, err := h.NeedsRehash(ctx, hash)
	if err != nil {
		t.Fatalf("NeedsRehash(current) error = %v", err)
	}

	if needed {
		t.Error("NeedsRehash(current) = true, want false")
	}
}

func conformanceInvalidHash(t *testing.T, factory func(t *testing.T) password.Hasher) {
	t.Helper()

	ctx := t.Context()
	h := factory(t)

	for _, bad := range []string{"", "not-a-hash", "$argon2id$v=19$m=1,t=1,p=1$c2FsdA$hash"} {
		if _, err := h.Verify(ctx, bad, "whatever"); !errors.Is(err, password.ErrInvalidHash) {
			t.Errorf("Verify(%q) err = %v, want ErrInvalidHash", bad, err)
		}

		if _, err := h.NeedsRehash(ctx, bad); !errors.Is(err, password.ErrInvalidHash) {
			t.Errorf("NeedsRehash(%q) err = %v, want ErrInvalidHash", bad, err)
		}
	}

	if _, err := h.Hash(ctx, strings.Repeat("p", 1025)); err == nil {
		t.Error("Hash(overlong) = nil, want ErrPasswordTooLong")
	} else if !errors.Is(err, password.ErrPasswordTooLong) {
		t.Errorf("Hash(overlong) err = %v, want ErrPasswordTooLong", err)
	}
}
