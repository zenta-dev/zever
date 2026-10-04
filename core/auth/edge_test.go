package auth

import (
	"errors"
	"testing"
)

func TestEdgeCloneValue_emptyCollections(t *testing.T) {
	t.Parallel()

	if got, ok := CloneValue(map[string]any{}).(map[string]any); !ok || got == nil || len(got) != 0 {
		t.Fatalf("CloneValue(empty map) = %#v, want non-nil empty", got)
	}

	if got, ok := CloneValue([]any{}).([]any); !ok || got == nil || len(got) != 0 {
		t.Fatalf("CloneValue(empty []any) = %#v, want non-nil empty", got)
	}

	if got, ok := CloneValue([]string{}).([]string); !ok || got == nil || len(got) != 0 {
		t.Fatalf("CloneValue(empty []string) = %#v, want non-nil empty", got)
	}

	if got, ok := CloneValue(map[string]string{}).(map[string]string); !ok || got == nil || len(got) != 0 {
		t.Fatalf("CloneValue(empty map[string]string) = %#v, want non-nil empty", got)
	}

	if got, ok := CloneValue([]byte{}).([]byte); !ok || got == nil || len(got) != 0 {
		t.Fatalf("CloneValue(empty []byte) = %#v, want non-nil empty", got)
	}
}

func TestEdgeClaims_Clone_emptyCustomStaysNonNil(t *testing.T) {
	t.Parallel()

	c := Claims{Subject: "u", Custom: map[string]any{}}
	cp := c.Clone()

	if cp.Custom == nil {
		t.Fatal("Clone of empty non-nil Custom = nil, want non-nil")
	}

	if len(cp.Custom) != 0 {
		t.Fatalf("Clone Custom len = %d, want 0", len(cp.Custom))
	}
}

func TestEdgeOptions_Validate_negativeTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"zero valid", Options{}, false},
		{"negative max ttl", Options{JWT: JWTOptions{MaxTTL: -1}}, true},
		{"negative leeway", Options{JWT: JWTOptions{Leeway: -1}}, true},
		{"negative oidc timeout", Options{OIDC: OIDCOptions{Timeout: -1}}, true},
		{"https issuer valid", Options{OIDC: OIDCOptions{Issuer: "https://issuer.example.com"}}, false},
		{"http issuer insecure valid", Options{OIDC: OIDCOptions{Issuer: "http://issuer.example.com", AllowInsecure: true}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.opts.Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}
