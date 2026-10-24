package auth

import (
	"testing"
	"time"
)

// BenchmarkOptionsValidate measures Options validation for an auth backend.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{
		JWT:  JWTOptions{MaxTTL: time.Hour, Leeway: time.Second},
		OIDC: OIDCOptions{Issuer: "https://idp.example.com", ClientID: "cid", Timeout: time.Second},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkClaimsClone measures the deep-copy hot path for claims.
func BenchmarkClaimsClone(b *testing.B) {
	c := Claims{
		Subject: "u1",
		Custom: map[string]any{
			"role": "admin",
			"tags": []string{"a", "b"},
			"meta": map[string]string{"k": "v"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		cp := c.Clone()
		if cp.Subject != c.Subject {
			b.Fatal("Clone dropped Subject")
		}
	}
}

// BenchmarkCloneValue measures the deep-copy hot path for claim values.
func BenchmarkCloneValue(b *testing.B) {
	v := map[string]any{
		"a": map[string]any{"b": "c"},
		"l": []any{"x", "y"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if CloneValue(v) == nil {
			b.Fatal("CloneValue returned nil")
		}
	}
}
