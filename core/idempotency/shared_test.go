package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKey_multibyteByteLengthBoundary(t *testing.T) {
	t.Parallel()
	multibyte := strings.Repeat("é", 127) + "a"
	if len(multibyte) != MaxKeyLen {
		t.Fatalf("test setup: len = %d, want %d", len(multibyte), MaxKeyLen)
	}
	if err := ValidateKey(multibyte); err != nil {
		t.Fatalf("ValidateKey(255 bytes multibyte) err = %v, want nil", err)
	}

	overBytes := strings.Repeat("é", 128)
	if len(overBytes) != MaxKeyLen+1 {
		t.Fatalf("test setup: len = %d, want %d", len(overBytes), MaxKeyLen+1)
	}
	if err := ValidateKey(overBytes); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey(256 bytes multibyte) err = %v, want ErrInvalidKey", err)
	}
}

func TestValidateKey_invalidUTF8_accepted(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("a\xffb"); err != nil {
		t.Fatalf("ValidateKey(invalid utf8) err = %v, want nil (RuneError skipped)", err)
	}
}
