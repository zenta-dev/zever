package slog

import (
	"io"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

// benchLogger builds a JSON logger over io.Discard so the handler encoding
// path is measured without destination noise.
func benchLogger(b *testing.B) log.Logger {
	b.Helper()

	return NewWithWriter(log.Options{MinLevel: log.LevelDebug}, io.Discard)
}

// BenchmarkNewWithWriter measures constructing the JSON handler and logger.
func BenchmarkNewWithWriter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, io.Discard); l == nil {
			b.Fatal("NewWithWriter returned nil")
		}
	}
}

// BenchmarkEmitSimple measures encoding a message with no attributes.
func BenchmarkEmitSimple(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().Msg("hello")
	}
}

// BenchmarkEmitWithFields measures encoding a message with several typed
// attributes.
func BenchmarkEmitWithFields(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().
			Str("method", "GET").
			Int("status", 200).
			Int64("bytes", 1024).
			Float64("latency", 1.5).
			Bool("cached", false).
			Dur("elapsed", time.Millisecond).
			Msg("request")
	}
}

// BenchmarkEmitInheritedFields measures emitting through a child logger
// built with fixed attributes.
func BenchmarkEmitInheritedFields(b *testing.B) {
	child := benchLogger(b).With().Str("service", "api").Int("version", 2).Logger()

	b.ReportAllocs()

	for b.Loop() {
		child.Info().Str("event", "hit").Msg("request")
	}
}

// BenchmarkEmitMsgf measures the formatted-message path.
func BenchmarkEmitMsgf(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().Int("n", 1).Msgf("count %d", 1)
	}
}

// BenchmarkEmitParallel measures concurrent JSON encoding.
func BenchmarkEmitParallel(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Info().Str("k", "v").Int("i", 1).Msg("parallel")
		}
	})
}

// BenchmarkEnabled measures the level check.
func BenchmarkEnabled(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		_ = l.Enabled(log.LevelInfo)
	}
}

// BenchmarkToSlogLevel measures mapping a zever level onto slog.
func BenchmarkToSlogLevel(b *testing.B) {
	levels := []log.Level{log.LevelDebug, log.LevelInfo, log.LevelWarn, log.LevelError, log.LevelFatal}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_ = toSlogLevel(levels[i%len(levels)])
		i++
	}
}

// BenchmarkWithContext measures attaching a context.
func BenchmarkWithContext(b *testing.B) {
	l := benchLogger(b)
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		_ = l.WithContext(ctx)
	}
}
