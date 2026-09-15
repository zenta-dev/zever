package flag

import (
	"errors"
	"strings"
	"testing"
)

func TestKey_Validate_valid_returnsNil(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"a", "my-flag", "flag_1", "feature.new-ui", "FLAG/123"} {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
}

func TestKey_Validate_empty_returnsInvalidKey(t *testing.T) {
	t.Parallel()
	err := ValidateKey("")
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey empty err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err %T is not *InvalidKeyError", err)
	}
	if ike.KeyLen != 0 {
		t.Errorf("KeyLen = %d want 0", ike.KeyLen)
	}
}

func TestKey_Validate_tooLong_returnsInvalidKey(t *testing.T) {
	t.Parallel()
	k := strings.Repeat("a", 257)
	err := ValidateKey(k)
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey 257-long err = %v, want ErrInvalidKey", err)
	}
	var ike *InvalidKeyError
	if !errors.As(err, &ike) {
		t.Fatalf("err %T is not *InvalidKeyError", err)
	}
	if ike.KeyLen != 257 {
		t.Errorf("KeyLen = %d want 257", ike.KeyLen)
	}
}

func TestKey_Validate_controlAndCRLF_rejected(t *testing.T) {
	t.Parallel()
	cases := []string{
		"flag\n",
		"flag\r",
		"flag\r\n",
		"fl\x00ag",
		"fl\x01ag",
		"fl\x1fag",
		"fl\x7fag",
		"fl\tag",
	}
	for _, k := range cases {
		if err := ValidateKey(k); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("ValidateKey(%q) = %v, want ErrInvalidKey", k, err)
		}
	}
	if err := ValidateKey("fl ag"); err != nil {
		t.Errorf("ValidateKey space err = %v, want nil", err)
	}
}
