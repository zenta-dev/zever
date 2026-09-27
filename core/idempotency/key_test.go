package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKey_valid_uuid_success(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatalf("ValidateKey(uuid) err = %v", err)
	}
}

func TestValidateKey_valid_random_success(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("order-12345_abc.DEF~token!ok"); err != nil {
		t.Fatalf("ValidateKey err = %v", err)
	}
}

func TestValidateKey_empty_fails(t *testing.T) {
	t.Parallel()
	err := ValidateKey("")
	if err == nil {
		t.Fatal("ValidateKey empty expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey empty err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err type = %T, want *InvalidKeyError", err)
	}
	if ike.KeyLen != 0 {
		t.Fatalf("InvalidKeyError.KeyLen = %d, want 0", ike.KeyLen)
	}
}

func TestValidateKey_overlong_256_fails(t *testing.T) {
	t.Parallel()
	key := strings.Repeat("a", 256)
	err := ValidateKey(key)
	if err == nil {
		t.Fatal("ValidateKey 256 expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err type = %T, want *InvalidKeyError", err)
	}
	if ike.KeyLen != 256 {
		t.Fatalf("InvalidKeyError.KeyLen = %d, want 256", ike.KeyLen)
	}
}

func TestValidateKey_maxlen_255_success(t *testing.T) {
	t.Parallel()
	if err := ValidateKey(strings.Repeat("b", 255)); err != nil {
		t.Fatalf("ValidateKey 255 err = %v", err)
	}
}

func TestValidateKey_crlf_rejected(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"a\rb", "a\nb", "a\r\nb"} {
		if err := ValidateKey(key); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("ValidateKey(%q) err = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestValidateKey_control_rejected(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"a\x00b", "a\x01b", "a\x7fb", "a\tb", "a\x1bb", "a\u0085b"} {
		if err := ValidateKey(key); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("ValidateKey(%q) err = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestValidateKey_error_never_echoes_key(t *testing.T) {
	t.Parallel()
	secret := "super-secret-key-material"
	err := ValidateKey(secret + "\n")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error message echoes key bytes: %q", err.Error())
	}
	longSecret := strings.Repeat("s", 300)
	err = ValidateKey(longSecret)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), longSecret) {
		t.Fatalf("error message echoes long key: %q", err.Error()[:64])
	}
}

func TestFingerprintMatches_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		stored   []byte
		incoming []byte
		want     bool
	}{
		{"nil_vs_nil", nil, nil, true},
		{"empty_vs_nil", []byte{}, nil, true},
		{"nil_vs_empty", nil, []byte{}, true},
		{"nil_vs_set", nil, []byte{1, 2, 3}, false},
		{"set_vs_nil", []byte{1, 2, 3}, nil, false},
		{"empty_vs_set", []byte{}, []byte{1}, false},
		{"set_vs_empty", []byte{1}, []byte{}, false},
		{"equal", []byte{1, 2, 3}, []byte{1, 2, 3}, true},
		{"different", []byte{1, 2, 3}, []byte{1, 2, 4}, false},
		{"different_len", []byte{1, 2}, []byte{1, 2, 3}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FingerprintMatches(tc.stored, tc.incoming); got != tc.want {
				t.Fatalf("FingerprintMatches(%v,%v) = %v, want %v", tc.stored, tc.incoming, got, tc.want)
			}
		})
	}
}
