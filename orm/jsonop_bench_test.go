package orm

import "testing"

// BenchmarkJSONKey measures building a single object-key JSONStep, the
// leaf of every JSON path expression.
func BenchmarkJSONKey(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = JSONKey("email")
	}
}
