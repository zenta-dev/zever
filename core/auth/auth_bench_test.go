package auth

import (
	"testing"
	"time"
)

// benchAdapter registers a stub auth backend once and returns its adapter so
// Open can be measured without paying the registration cost.
func benchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Auth, error) { return &stubAuth{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(Options) (Auth, error) { return &stubAuth{}, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a := freshAdapter()
		if err := Register(a, factory); err != nil {
			b.Fatalf("Register(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpen measures the registry lookup plus factory invocation hot path.
func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpenParallel measures concurrent Open calls on one adapter.
func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Errorf("Open(%v) err = %v", a, err)
				return
			}
		}
	})
}

// BenchmarkOptionsValidate measures options validation with OIDC issuer present.
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

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ParseAdapter("jwt"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
		}
	}
}
