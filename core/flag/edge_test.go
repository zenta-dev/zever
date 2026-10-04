package flag

import (
	"strings"
	"testing"
)

func TestEdgeValidateKey_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"empty", "", true},
		{"single byte", "a", false},
		{"max length", strings.Repeat("a", 256), false},
		{"over max", strings.Repeat("a", 257), true},
		{"internal space", "a b", false},
		{"tab control", "a\tb", true},
		{"del", "a\x7fb", true},
		{"nul", "a\x00b", true},
		{"multibyte unicode", "café", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateKey(tc.key)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateKey(len=%d) err = nil, want error", len(tc.key))
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateKey(%q) err = %v, want nil", tc.key, err)
			}
		})
	}
}

func TestEdgeGetJSON_zeroFallback(t *testing.T) {
	t.Parallel()

	f := &jsonStubFlag{values: map[string]any{}}

	got, err := GetJSON(t.Context(), f, "missing", 0)
	if err != nil {
		t.Fatalf("GetJSON err = %v, want nil", err)
	}

	if got != 0 {
		t.Fatalf("GetJSON = %d, want 0", got)
	}
}
