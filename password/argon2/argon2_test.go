package argon2

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/password"
)

func testHasher(t *testing.T) password.Hasher {
	t.Helper()

	h, err := New(password.Options{
		Time:    1,
		Memory:  8 * 1024,
		Threads: 1,
		SaltLen: 8,
		KeyLen:  16,
	})
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}

	return h
}

func TestHashVerifyRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pw   string
	}{
		{name: "simple", pw: "correct horse battery staple"},
		{name: "empty", pw: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			h := testHasher(t)

			hash, err := h.Hash(ctx, tc.pw)
			if err != nil {
				t.Fatalf("Hash() = %v, want nil", err)
			}

			if !strings.HasPrefix(hash, "$argon2id$v=19$") {
				t.Fatalf("Hash() = %q, want $argon2id$v=19$ prefix", hash)
			}

			ok, err := h.Verify(ctx, hash, tc.pw)
			if err != nil {
				t.Fatalf("Verify() = %v, want nil", err)
			}

			if !ok {
				t.Fatal("Verify() = false, want true")
			}
		})
	}
}

func TestPasswordLengthLimits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	pw1024 := strings.Repeat("a", 1024)
	hash, err := h.Hash(ctx, pw1024)
	if err != nil {
		t.Fatalf("Hash(1024) = %v, want nil", err)
	}

	ok, err := h.Verify(ctx, hash, pw1024)
	if err != nil {
		t.Fatalf("Verify(1024) = %v, want nil", err)
	}

	if !ok {
		t.Fatal("Verify(1024) = false, want true")
	}

	pw1025 := strings.Repeat("a", 1025)

	cases := []struct {
		name string
		run  func() error
	}{
		{name: "hash rejects 1025", run: func() error { _, err := h.Hash(ctx, pw1025); return err }},
		{name: "verify rejects 1025", run: func() error { _, err := h.Verify(ctx, hash, pw1025); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.run()
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if !errors.Is(err, password.ErrPasswordTooLong) {
				t.Fatalf("errors.Is(%v, ErrPasswordTooLong) = false", err)
			}
		})
	}
}

func TestVerifyMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	hash, err := h.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	ok, err := h.Verify(ctx, hash, "wrong")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false")
	}
}

func TestVerifyInvalidHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	_, err := h.Verify(ctx, "not-a-hash", "secret")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !errors.Is(err, password.ErrInvalidHash) {
		t.Fatalf("errors.Is(%v, ErrInvalidHash) = false", err)
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	hash, err := h.Hash(ctx, "correct")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	ok, err := h.Verify(ctx, hash, "incorrect")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false")
	}
}

func TestVerifyTamperedHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	hash, err := h.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}

	// Tamper the first char of the hash segment: it encodes 6 full data
	// bits, so any change alters the decoded tag. (The last char only
	// carries 2 data bits — flipping it can decode to identical bytes.)
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("Hash() = %q, want 6 PHC segments", hash)
	}

	first := parts[5][0]
	replacement := byte('A')
	if first == 'A' {
		replacement = 'B'
	}

	parts[5] = string(replacement) + parts[5][1:]
	tampered := strings.Join(parts, "$")

	ok, err := h.Verify(ctx, tampered, "secret")
	if err != nil {
		t.Fatalf("Verify(tampered) = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify(tampered) = true, want false")
	}
}

func TestHashRandFailure(t *testing.T) {
	// Serial on purpose: mutates global randRead, races with parallel Hash calls.
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom") }

	defer func() { randRead = old }()

	ctx := context.Background()
	h := testHasher(t)

	_, err := h.Hash(ctx, "secret")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !strings.Contains(err.Error(), "generate salt") {
		t.Fatalf("err = %v, want salt generation wrapper", err)
	}
}

func TestCrossParamsVerify(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reduced := testHasher(t)

	hash, err := reduced.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	def, err := New(password.Options{})
	if err != nil {
		t.Fatalf("New(defaults) = %v, want nil", err)
	}

	ok, err := def.Verify(ctx, hash, "secret")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if !ok {
		t.Fatal("Verify() cross-params = false, want true")
	}
}

func TestNeedsRehash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	hash, err := h.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	ok, err := h.NeedsRehash(ctx, hash)
	if err != nil {
		t.Fatalf("NeedsRehash() = %v, want nil", err)
	}

	if ok {
		t.Fatal("NeedsRehash() = true, want false")
	}

	cases := []struct {
		name string
		opts password.Options
	}{
		{name: "time drift", opts: password.Options{Time: 2, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16}},
		{name: "memory drift", opts: password.Options{Time: 1, Memory: 16 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16}},
		{name: "threads drift", opts: password.Options{Time: 1, Memory: 8 * 1024, Threads: 2, SaltLen: 8, KeyLen: 16}},
		{name: "keylen drift", opts: password.Options{Time: 1, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 32}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			drifted, err := New(tc.opts)
			if err != nil {
				t.Fatalf("New() = %v, want nil", err)
			}

			ok, err := drifted.NeedsRehash(ctx, hash)
			if err != nil {
				t.Fatalf("NeedsRehash() = %v, want nil", err)
			}

			if !ok {
				t.Fatal("NeedsRehash() = false, want true")
			}
		})
	}
}

func TestNeedsRehashInvalid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := testHasher(t)

	_, err := h.NeedsRehash(ctx, "not-a-hash")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !errors.Is(err, password.ErrInvalidHash) {
		t.Fatalf("errors.Is(%v, ErrInvalidHash) = false", err)
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	t.Parallel()

	params, salt, hash, err := decodeHash(encodeHash(1, 8*1024, 2, []byte("12345678"), []byte("1234567890123456")))
	if err != nil {
		t.Fatalf("decodeHash() = %v, want nil", err)
	}

	if params.time != 1 || params.memory != 8*1024 || params.threads != 2 {
		t.Fatalf("params = %+v, want t=1 m=8192 p=2", params)
	}

	if string(salt) != "12345678" {
		t.Fatalf("salt = %q, want %q", salt, "12345678")
	}

	if string(hash) != "1234567890123456" {
		t.Fatalf("hash = %q, want 16-byte tag", hash)
	}
}

func TestDecodeHashTable(t *testing.T) {
	t.Parallel()

	t.Helper()
	validSalt := "MTIzNDU2Nzg"            // "12345678"
	validHash := "MTIzNDU2Nzg5MDEyMzQ1Ng" // 16 bytes

	prefix := func(params string) string {
		return "$argon2id$v=19$" + params + "$" + validSalt + "$" + validHash
	}

	cases := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "5 parts", input: "$argon2id$v=19$m=8192,t=1,p=1$" + validSalt},
		{name: "7 parts", input: prefix("m=8192,t=1,p=1") + "$extra"},
		{name: "wrong alg", input: "$argon2i$v=19$m=8192,t=1,p=1$" + validSalt + "$" + validHash},
		{name: "wrong version", input: "$argon2id$v=16$m=8192,t=1,p=1$" + validSalt + "$" + validHash},
		{name: "empty params", input: "$argon2id$v=19$$" + validSalt + "$" + validHash},
		{name: "empty salt", input: "$argon2id$v=19$m=8192,t=1,p=1$$" + validHash},
		{name: "empty hash", input: "$argon2id$v=19$m=8192,t=1,p=1$" + validSalt + "$"},
		{name: "unknown key", input: prefix("m=8192,t=1,p=1,x=2")},
		{name: "missing m", input: prefix("t=1,p=1")},
		{name: "missing t", input: prefix("m=8192,p=1")},
		{name: "missing p", input: prefix("m=8192,t=1")},
		{name: "bad uint m", input: prefix("m=abc,t=1,p=1")},
		{name: "bad uint t", input: prefix("m=8192,t=abc,p=1")},
		{name: "bad uint p", input: prefix("m=8192,t=1,p=abc")},
		{name: "bare key without equals", input: prefix("m8192,t=1,p=1")},
		{name: "salt too short", input: "$argon2id$v=19$m=8192,t=1,p=1$QUJD$" + validHash},
		{name: "hash too short", input: "$argon2id$v=19$m=8192,t=1,p=1$" + validSalt + "$QUJD"},
		{name: "bad base64 salt", input: "$argon2id$v=19$m=8192,t=1,p=1$!!!$" + validHash},
		{name: "bad base64 hash", input: "$argon2id$v=19$m=8192,t=1,p=1$" + validSalt + "$!!!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := decodeHash(tc.input)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if !errors.Is(err, password.ErrInvalidHash) {
				t.Fatalf("errors.Is(%v, ErrInvalidHash) = false", err)
			}
		})
	}
}

// TestRegistryIntegration is the sole serial test: it mutates the global
// registry and performs the single full-default roundtrip.
func TestRegistryIntegration(t *testing.T) {
	ctx := context.Background()

	if err := password.Register(password.AdapterArgon2ID, New); err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}

	reduced, err := password.Open(password.AdapterArgon2ID, password.Options{
		Time:    1,
		Memory:  8 * 1024,
		Threads: 1,
		SaltLen: 8,
		KeyLen:  16,
	})
	if err != nil {
		t.Fatalf("Open() = %v, want nil", err)
	}

	hash, err := reduced.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	ok, err := reduced.Verify(ctx, hash, "secret")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if !ok {
		t.Fatal("Verify() = false, want true")
	}

	defHash, err := password.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("password.Hash() = %v, want nil", err)
	}

	ok, err = password.Verify(ctx, defHash, "secret")
	if err != nil {
		t.Fatalf("password.Verify() = %v, want nil", err)
	}

	if !ok {
		t.Fatal("password.Verify() = false, want true")
	}

	ok, err = password.Verify(ctx, defHash, "wrong")
	if err != nil {
		t.Fatalf("password.Verify(wrong) = %v, want nil", err)
	}

	if ok {
		t.Fatal("password.Verify(wrong) = true, want false")
	}
}
