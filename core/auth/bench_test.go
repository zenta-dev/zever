package auth

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Auth, error) { return stubAuth{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Auth, error) { return stubAuth{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkClaimsClone(b *testing.B) {
	c := Claims{
		Subject: "user-1",
		Custom: map[string]any{
			"role":   "admin",
			"nested": map[string]any{"key": "val"},
			"list":   []any{"a", map[string]any{"deep": "x"}},
			"tags":   []string{"a", "b"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.Clone()
	}
}

func BenchmarkCloneValue(b *testing.B) {
	v := map[string]any{
		"role":   "admin",
		"nested": map[string]any{"key": "val"},
		"list":   []any{"a", "b", "c"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = CloneValue(v)
	}
}
