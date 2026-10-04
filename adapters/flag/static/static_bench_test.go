package static

import "testing"

// benchDriver builds a reload-disabled driver over an in-memory flag set so
// benchmarks isolate the lookup and coercion hot path.
func benchDriver(b *testing.B) *driver {
	b.Helper()

	return &driver{flags: map[string]any{
		"enabled": true,
		"name":    "zever",
		"count":   float64(42),
		"ratio":   3.5,
	}}
}

// BenchmarkBool measures key validation, lookup, and bool coercion.
func BenchmarkBool(b *testing.B) {
	d := benchDriver(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Bool(ctx, "enabled", false); err != nil {
			b.Fatalf("Bool: %v", err)
		}
	}
}

// BenchmarkString measures key validation, lookup, and string coercion.
func BenchmarkString(b *testing.B) {
	d := benchDriver(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.String(ctx, "name", ""); err != nil {
			b.Fatalf("String: %v", err)
		}
	}
}

// BenchmarkInt measures key validation, lookup, and float-to-int coercion.
func BenchmarkInt(b *testing.B) {
	d := benchDriver(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Int(ctx, "count", 0); err != nil {
			b.Fatalf("Int: %v", err)
		}
	}
}
