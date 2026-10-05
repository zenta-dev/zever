package slog

import (
	"encoding/json"
	"io"
	"testing"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/core/log"
)

func TestEdge_ToSlogLevelAll(t *testing.T) {
	t.Parallel()

	cases := map[log.Level]stdslog.Level{
		log.LevelDebug: stdslog.LevelDebug,
		log.LevelInfo:  stdslog.LevelInfo,
		log.LevelWarn:  stdslog.LevelWarn,
		log.LevelError: stdslog.LevelError,
		log.LevelFatal: fatalLevel,
	}

	for in, want := range cases {
		if got := toSlogLevel(in); got != want {
			t.Errorf("toSlogLevel(%v) = %v, want %v", in, got, want)
		}
	}

	if got := toSlogLevel(log.Level(99)); got != stdslog.LevelInfo {
		t.Errorf("toSlogLevel(unknown) = %v, want info", got)
	}
}

func TestEdge_FatalLevelAboveError(t *testing.T) {
	t.Parallel()

	if fatalLevel <= stdslog.LevelError {
		t.Fatalf("fatalLevel = %v, want above %v", fatalLevel, stdslog.LevelError)
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

func TestEdge_ContextNilErrorsSkipped(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)

	l.With().Err(nil).AnErr("cause", nil).Logger().Info().Msg("m")

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

func TestEdge_WithContextFallsBack(t *testing.T) {
	t.Parallel()

	l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, io.Discard)

	if got := l.WithContext(t.Context()); got == nil {
		t.Fatal("WithContext(empty) = nil, want fallback logger")
	}
}

func TestEdge_SendEmptyMsg(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)

	l.Info().Str("k", "v").Send()

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["msg"] != "" {
		t.Errorf("msg = %v, want empty", got["msg"])
	}
}
