package pretty

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
)

// benchLogger builds a color-free logger writing to io.Discard so the
// render path is measured without terminal or allocation noise from the
// destination.
func benchLogger(b *testing.B) *logger {
	b.Helper()

	return &logger{
		mu:         &sync.Mutex{},
		out:        io.Discard,
		minLevel:   log.LevelDebug,
		timeFormat: defaultTimeFormat,
	}
}

// BenchmarkNewWithWriter measures constructing a logger over a discard
// writer.
func BenchmarkNewWithWriter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, io.Discard); l == nil {
			b.Fatal("NewWithWriter returned nil")
		}
	}
}

// BenchmarkEmitSimple measures rendering a message with no fields.
func BenchmarkEmitSimple(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().Msg("hello")
	}
}

// BenchmarkEmitWithFields measures rendering a message with several typed
// fields, including the field sort and value formatting.
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
// that carries fixed context fields.
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

// BenchmarkEmitParallel measures concurrent rendering, which serializes on
// the shared writer mutex.
func BenchmarkEmitParallel(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Info().Str("k", "v").Int("i", 1).Msg("parallel")
		}
	})
}

// BenchmarkFormatField measures rendering one field of each type.
func BenchmarkFormatField(b *testing.B) {
	fields := []log.Field{
		log.String("s", "value"),
		log.Int("i", 1),
		log.Int64("i64", 2),
		log.Float64("f", 1.5),
		log.Bool("b", true),
		log.Duration("d", time.Second),
		log.Time("t", time.Unix(0, 0)),
		log.Err(nil),
		log.Any("a", 42),
	}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_ = formatField(fields[i%len(fields)])
		i++
	}
}

// BenchmarkWriteFields measures sorting and appending a field set.
func BenchmarkWriteFields(b *testing.B) {
	fields := []log.Field{
		log.String("method", "GET"),
		log.Int("status", 200),
		log.Bool("cached", false),
	}

	b.ReportAllocs()

	for b.Loop() {
		var sb strings.Builder

		writeFields(&sb, fields, false)
	}
}

// BenchmarkWithContextRequestID measures attaching a request id and
// emitting.
func BenchmarkWithContextRequestID(b *testing.B) {
	l := benchLogger(b)
	ctx := observability.WithRequestID(b.Context(), "req1234")

	b.ReportAllocs()

	for b.Loop() {
		l.WithContext(ctx).Info().Msg("request")
	}
}

// BenchmarkEnabled measures the level check.
func BenchmarkEnabled(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		_ = l.Enabled(log.LevelInfo)
	}
}

// BenchmarkSync measures the no-op flush.
func BenchmarkSync(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		if err := l.Sync(); err != nil {
			b.Fatal("Sync() =", err)
		}
	}
}

// BenchmarkName measures the adapter name lookup.
func BenchmarkName(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		if got := l.Name(); got != "pretty" {
			b.Fatal("Name() =", got)
		}
	}
}

// BenchmarkSend measures the bare Send dispatch.
func BenchmarkSend(b *testing.B) {
	l := benchLogger(b)

	b.ReportAllocs()

	for b.Loop() {
		l.Info().Str("k", "v").Send()
	}
}

// BenchmarkResolveMinLevel measures validating a configured minimum level.
func BenchmarkResolveMinLevel(b *testing.B) {
	levels := []log.Level{log.LevelDebug, log.LevelInfo, log.LevelWarn, log.LevelError, log.LevelFatal}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_ = resolveMinLevel(levels[i%len(levels)])
		i++
	}
}

// BenchmarkLevelStyle measures mapping a level onto its label and color.
func BenchmarkLevelStyle(b *testing.B) {
	levels := []log.Level{log.LevelDebug, log.LevelInfo, log.LevelWarn, log.LevelError, log.LevelFatal}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_, _ = levelStyle(levels[i%len(levels)])
		i++
	}
}

// BenchmarkIsCharDevice measures the writer character-device probe.
func BenchmarkIsCharDevice(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = isCharDevice(io.Discard)
	}
}

// BenchmarkNew measures constructing a logger over stdout.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if l := New(log.Options{MinLevel: log.LevelDebug}); l == nil {
			b.Fatal("New() = nil")
		}
	}
}
