package zerolog

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	zl "github.com/rs/zerolog"

	"github.com/zenta-dev/zever/core/log"
)

func TestEdge_ToZeroLogLevelAll(t *testing.T) {
	t.Parallel()

	cases := map[log.Level]zl.Level{
		log.LevelDebug: zl.DebugLevel,
		log.LevelInfo:  zl.InfoLevel,
		log.LevelWarn:  zl.WarnLevel,
		log.LevelError: zl.ErrorLevel,
		log.LevelFatal: zl.FatalLevel,
	}

	for in, want := range cases {
		if got := toZeroLogLevel(in); got != want {
			t.Errorf("toZeroLogLevel(%v) = %v, want %v", in, got, want)
		}
	}

	if got := toZeroLogLevel(log.Level(99)); got != zl.InfoLevel {
		t.Errorf("toZeroLogLevel(unknown) = %v, want info", got)
	}
}

func TestEdge_EnabledBoundaries(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{MinLevel: log.LevelWarn}, io.Discard)

	for _, tc := range []struct {
		level log.Level
		want  bool
	}{
		{log.LevelDebug, false},
		{log.LevelInfo, false},
		{log.LevelWarn, true},
		{log.LevelError, true},
		{log.LevelFatal, true},
	} {
		if got := l.Enabled(tc.level); got != tc.want {
			t.Errorf("Enabled(%v) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

func TestEdge_WithContextPreservesLevel(t *testing.T) {
	t.Parallel()

	got := NewWithWriter(log.Options{MinLevel: log.LevelWarn}, io.Discard)

	a, ok := got.(*zerologAdapter)
	if !ok {
		t.Fatalf("logger type = %T, want *zerologAdapter", got)
	}

	// WithContext reads the zerolog logger stored in ctx; seed it via the
	// adapter's own logger so the child keeps the configured level.
	ctx := a.log.WithContext(t.Context())

	child := a.WithContext(ctx)
	if child.Enabled(log.LevelInfo) {
		t.Error("Enabled(info) = true, want false on level-preserving child")
	}

	if !child.Enabled(log.LevelWarn) {
		t.Error("Enabled(warn) = false, want true on level-preserving child")
	}
}

func TestEdge_ContextLoggerFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.With().Str("service", "api").Int("version", 2).Logger().Info().Msg("child")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["service"] != "api" || got["version"] != float64(2) {
		t.Fatalf("context fields missing: %v", got)
	}
}

func TestEdge_EventNilErrors(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.Info().Err(nil).AnErr("cause", nil).Msg("m")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	for _, key := range []string{"error", "cause"} {
		if _, ok := got[key]; ok {
			t.Errorf("key %q present, want skipped: %v", key, got)
		}
	}
}

func TestEdge_SendEmptyMsg(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.Info().Str("k", "v").Send()

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if _, ok := got["k"]; !ok {
		t.Fatalf("field k missing: %v", got)
	}
}

func TestEdge_NilWriterFallback(t *testing.T) {
	t.Parallel()

	if l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, nil); l == nil {
		t.Fatal("NewWithWriter(nil) = nil, want logger")
	}
}

func TestEdge_FilteredLevelNoOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelError}, &buf)

	l.Debug().Msg("skip")
	l.Info().Msg("skip")
	l.Warn().Msg("skip")

	if buf.Len() != 0 {
		t.Fatalf("output = %q, want empty for levels below min", buf.String())
	}

	l.Error().Msg("keep")

	if !strings.Contains(buf.String(), "keep") {
		t.Errorf("output = %q, want error line kept", buf.String())
	}
}

func TestEdge_ConcurrentEmit(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, io.Discard)

	const goroutines = 32

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()

			l.Info().Int("i", i).Msg("concurrent")
		}(i)
	}

	wg.Wait()
}

func TestEdge_DuplicateKeys(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.Info().Str("k", "first").Str("k", "second").Msg("dup")

	if got := buf.String(); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("output = %q, want both duplicate values", got)
	}
}

func TestEdge_EnabledUnknownLevel(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{MinLevel: log.LevelWarn}, io.Discard)

	if l.Enabled(log.Level(99)) {
		t.Error("Enabled(unknown) = true, want false (unknown maps to info, below warn)")
	}

	l2 := NewWithWriter(log.Options{MinLevel: log.LevelInfo}, io.Discard)

	if !l2.Enabled(log.Level(99)) {
		t.Error("Enabled(unknown) at info min = false, want true (unknown maps to info fallback)")
	}
}

func TestEdge_ContextLoggerNoArgs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, &buf)
	l.With().Logger().Info().Msg("no-args")

	if !strings.Contains(buf.String(), "no-args") {
		t.Errorf("output = %q, want no-args line", buf.String())
	}
}

func TestEdge_WithContextBackground(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{MinLevel: log.LevelWarn}, io.Discard)

	child := l.WithContext(t.Context())

	if child == nil {
		t.Fatal("WithContext(bg) = nil, want logger")
	}

	if child.Enabled(log.LevelInfo) {
		t.Error("Enabled(info) = true, want false on background child")
	}
}
