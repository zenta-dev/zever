package noop

import "testing"

// BenchmarkLog measures the discarded-event path through the full chain.
func BenchmarkLog(b *testing.B) {
	l := New()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l.Info().Str("key", "value").Int("n", 1).Msg("hello")
	}
}
