package ratelimit

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateKey_boundary_lengths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"one byte", "a", false},
		{"max len", strings.Repeat("a", MaxKeyLen), false},
		{"max len plus one", strings.Repeat("a", MaxKeyLen+1), true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateKey(tc.key)
			if tc.wantErr && !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("ValidateKey(len=%d) err = %v, want ErrInvalidKey", len(tc.key), err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateKey(len=%d) err = %v, want nil", len(tc.key), err)
			}
		})
	}
}

func TestDecision_zero(t *testing.T) {
	t.Parallel()

	var d Decision
	if d.Allowed || d.RetryAfter != 0 || d.Remaining != 0 {
		t.Fatalf("zero Decision = %+v, want zero", d)
	}
}

