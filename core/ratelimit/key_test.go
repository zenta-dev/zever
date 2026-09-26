package ratelimit

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKey_valid(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"user:123", "192.168.1.1", "a", "api-key_~v2", strings.Repeat("a", MaxKeyLen)} {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) err = %v, want nil", k, err)
		}
	}
}

func TestValidateKey_empty_isInvalid(t *testing.T) {
	t.Parallel()
	err := ValidateKey("")
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("empty err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err %T is not *InvalidKeyError", err)
	}
	if ike.KeyLen != 0 {
		t.Errorf("KeyLen = %d want 0", ike.KeyLen)
	}
}

func TestValidateKey_tooLong_isInvalid(t *testing.T) {
	t.Parallel()
	k := strings.Repeat("a", MaxKeyLen+1)
	err := ValidateKey(k)
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("257-long err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err %T is not *InvalidKeyError", err)
	}
	if ike.KeyLen != MaxKeyLen+1 {
		t.Errorf("KeyLen = %d want %d", ike.KeyLen, MaxKeyLen+1)
	}
}

func TestValidateKey_controlsAndCRLF_areInvalid(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"a\nb", "a\rb", "a\r\nb", "ab\x00cd", "ab\x1fcd", "ab\x7fcd", "tab\there"} {
		if err := ValidateKey(k); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("ValidateKey(%q) err = %v, want ErrInvalidKey", k, err)
		}
	}
}

func TestValidateKey_neverEchoesKey(t *testing.T) {
	t.Parallel()
	secret := "10.0.0.1-secret\r\n"
	err := ValidateKey(secret)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "10.0.0.1-secret") {
		t.Errorf("error %q echoes PII key", err.Error())
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err %T is not *InvalidKeyError", err)
	}
	if ike.KeyLen != len(secret) {
		t.Errorf("KeyLen = %d want %d", ike.KeyLen, len(secret))
	}
}
