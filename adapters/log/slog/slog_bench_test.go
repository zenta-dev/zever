package slog

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

// BenchmarkLog measures JSON encoding and writing one info line to io.Discard.
func BenchmarkLog(b *testing.B) {
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, io.Discard)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l.Info().Str("key", "value").Int("n", 1).Msg("hello")
	}
}

// BenchmarkLogParallel measures concurrent writes through the slog handler.
func BenchmarkLogParallel(b *testing.B) {
	l := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, io.Discard)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Info().Str("key", "value").Int("n", 1).Msg("hello")
		}
	})
}
