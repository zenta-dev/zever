package idempotency

import (
	"errors"
	"testing"
	"time"

	zredis "github.com/zenta-dev/zever/internal/redis"
)

func TestCoverDuplicateErrorString(t *testing.T) {
	t.Parallel()
	err := &DuplicateError{Adapter: Memory}
	if got, want := err.Error(), "idempotency: duplicate registration: memory"; got != want {
		t.Fatalf("DuplicateError.Error() = %q, want %q", got, want)
	}
}

func TestCoverUnknownAdapterErrorString(t *testing.T) {
	t.Parallel()
	err := &UnknownAdapterError{Adapter: Redis}
	if got, want := err.Error(), "idempotency: unknown adapter: redis (forgotten import?)"; got != want {
		t.Fatalf("UnknownAdapterError.Error() = %q, want %q", got, want)
	}
}

func TestCoverValidateKeyRuneErrorSkip(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("ab\uFFFDb"); err != nil {
		t.Fatalf("ValidateKey with U+FFFD err = %v, want nil", err)
	}
}

func TestCoverValidateKeyRawByteC0(t *testing.T) {
	t.Parallel()
	err := ValidateKey("\xc3\x01")
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey(%q) err = %v, want ErrInvalidKey", "\xc3\x01", err)
	}
}

func TestCoverOptionsTTLSet(t *testing.T) {
	t.Parallel()
	if got := (Options{TTL: 5 * time.Second}).ttl(); got != 5*time.Second {
		t.Fatalf("ttl() = %v, want %v", got, 5*time.Second)
	}
}

func TestCoverIsTokenCharTable(t *testing.T) {
	t.Parallel()
	for _, c := range []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		if !zredis.IsTokenChar(c) {
			t.Fatalf("IsTokenChar(%q) = false, want true", c)
		}
	}
	for _, c := range []byte{'!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~'} {
		if !zredis.IsTokenChar(c) {
			t.Fatalf("IsTokenChar(%q) = false, want true", c)
		}
	}
	for _, c := range []byte{'(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}', ' ', '\t', 0x00, 0x1f, 0x7f, 0x80} {
		if zredis.IsTokenChar(c) {
			t.Fatalf("IsTokenChar(%q) = true, want false", c)
		}
	}
}
