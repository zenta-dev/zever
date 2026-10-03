package orm

import "testing"

// BenchmarkEscapeLike measures the LIKE-pattern escaping hot path. The
// replacer is hoisted to a package-level var, so this allocates only the
// result string, not a per-call strings.Replacer.
func BenchmarkEscapeLike(b *testing.B) {
	s := `100%_safe\path`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = escapeLike(s)
	}
}
