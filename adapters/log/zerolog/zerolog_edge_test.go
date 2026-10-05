package zerolog

import (
	"bytes"
	"encoding/json"
	"io"
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
