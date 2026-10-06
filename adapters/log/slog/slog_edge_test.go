package slog

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
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

func TestEdge_NilWriterFallsBackToStdout(t *testing.T) {
	t.Parallel()

	if l := NewWithWriter(log.Options{MinLevel: log.LevelDebug}, nil); l == nil {
		t.Fatal("NewWithWriter(nil) = nil, want logger")
	}
}

func TestEdge_FilteredLevelNoOutput(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelError)

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

	l, _ := newBuffered(log.LevelDebug)

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

	l, buf := newBuffered(log.LevelDebug)

	l.Info().Str("k", "first").Str("k", "second").Msg("dup")

	if got := buf.String(); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("output = %q, want both duplicate values", got)
	}
}

func TestEdge_EnabledUnknownLevel(t *testing.T) {
	t.Parallel()

	l, _ := newBuffered(log.LevelInfo)

	if !l.Enabled(log.Level(99)) {
		t.Error("Enabled(unknown) = false, want true (unknown maps to info fallback)")
	}

	l2, _ := newBuffered(log.LevelError)

	if l2.Enabled(log.Level(99)) {
		t.Error("Enabled(unknown) at error min = true, want false (unknown maps to info, below error)")
	}
}

func TestEdge_ContextLoggerNoArgs(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)

	l.With().Logger().Info().Msg("no-args")

	if !strings.Contains(buf.String(), "no-args") {
		t.Errorf("output = %q, want no-args line", buf.String())
	}
}
