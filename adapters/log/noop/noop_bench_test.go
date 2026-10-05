package noop

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

// BenchmarkNew measures constructing the no-op logger.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if l := New(); l == nil {
			b.Fatal("New returned nil")
		}
	}
}

// BenchmarkEventChain measures building and discarding a full event with
// every field type.
func BenchmarkEventChain(b *testing.B) {
	l := New()
	ts := time.Unix(0, 0)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().
			Str("s", "v").
			Int("i", 1).
			Int64("i64", 2).
			Float64("f", 1.5).
			Bool("b", true).
			Dur("d", time.Second).
			Time("t", ts).
			Err(nil).
			AnErr("e", nil).
			Any("a", nil).
			Msg("done")
	}
}

// BenchmarkMsgf measures the formatted-message path.
func BenchmarkMsgf(b *testing.B) {
	l := New()

	b.ReportAllocs()

	for b.Loop() {
		l.Info().Int("n", 1).Msgf("count %d", 1)
	}
}

// BenchmarkWith measures building a child logger from a context chain.
func BenchmarkWith(b *testing.B) {
	l := New()

	b.ReportAllocs()

	for b.Loop() {
		_ = l.With().Str("service", "api").Int("n", 1).Logger()
	}
}

// BenchmarkWithContext measures attaching a context.
func BenchmarkWithContext(b *testing.B) {
	l := New()
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		_ = l.WithContext(ctx)
	}
}

// BenchmarkEnabled measures the level check, always false for noop.
func BenchmarkEnabled(b *testing.B) {
	l := New()

	b.ReportAllocs()

	for b.Loop() {
		_ = l.Enabled(log.LevelInfo)
	}
}

// BenchmarkContextLogger measures materializing a logger from a context.
func BenchmarkContextLogger(b *testing.B) {
	c := New().With()

	b.ReportAllocs()

	for b.Loop() {
		_ = c.Logger()
	}
}

// BenchmarkParallelEmit measures concurrent event emission.
func BenchmarkParallelEmit(b *testing.B) {
	l := New()

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Info().Str("k", "v").Int("i", 1).Msg("parallel")
		}
	})
}
