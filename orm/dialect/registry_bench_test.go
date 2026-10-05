package dialect

import "testing"

// BenchmarkFor measures the read-locked registry lookup on the hot path
// every render call takes to obtain its Dialect.
func BenchmarkFor(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		d, err := For("sqlite")
		if err != nil {
			b.Fatalf("For: %v", err)
		}

		if d.Name() != "sqlite" {
			b.Fatalf("Name() = %q, want sqlite", d.Name())
		}
	}
}
