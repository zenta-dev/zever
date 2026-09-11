package slog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

var (
	_ log.Logger  = (*slogLogger)(nil)
	_ log.Event   = (*slogEvent)(nil)
	_ log.Context = (*slogContext)(nil)
)

func newBuffered(minLevel log.Level) (*slogLogger, *bytes.Buffer) {
	var buf bytes.Buffer
	handler := stdslog.NewJSONHandler(&buf, &stdslog.HandlerOptions{Level: toSlogLevel(minLevel)})
	return &slogLogger{logger: stdslog.New(handler)}, &buf
}

func TestNew_enabledMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		minLevel log.Level
		checks   map[log.Level]bool
	}{
		{name: "debug enables all", minLevel: log.LevelDebug, checks: map[log.Level]bool{log.LevelDebug: true, log.LevelInfo: true, log.LevelWarn: true, log.LevelError: true, log.LevelFatal: true}},
		{name: "info disables debug", minLevel: log.LevelInfo, checks: map[log.Level]bool{log.LevelDebug: false, log.LevelInfo: true, log.LevelWarn: true, log.LevelError: true, log.LevelFatal: true}},
		{name: "warn", minLevel: log.LevelWarn, checks: map[log.Level]bool{log.LevelDebug: false, log.LevelInfo: false, log.LevelWarn: true, log.LevelError: true, log.LevelFatal: true}},
		{name: "error", minLevel: log.LevelError, checks: map[log.Level]bool{log.LevelDebug: false, log.LevelInfo: false, log.LevelWarn: false, log.LevelError: true, log.LevelFatal: true}},
		{name: "fatal only", minLevel: log.LevelFatal, checks: map[log.Level]bool{log.LevelDebug: false, log.LevelInfo: false, log.LevelWarn: false, log.LevelError: false, log.LevelFatal: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := New(log.Options{MinLevel: tt.minLevel})
			for lv, want := range tt.checks {
				if got := l.Enabled(lv); got != want {
					t.Errorf("Enabled(%v) = %v, want %v", lv, got, want)
				}
			}
		})
	}
}

func TestNew_unknownMinLevel_fallsBackToInfo(t *testing.T) {
	t.Parallel()

	l := New(log.Options{MinLevel: log.Level(99)})

	if l.Enabled(log.LevelDebug) {
		t.Error("Enabled(debug) = true, want false")
	}

	if !l.Enabled(log.LevelInfo) {
		t.Error("Enabled(info) = false, want true")
	}
}

func TestAdapter_identity(t *testing.T) {
	t.Parallel()

	l := New(log.Options{})

	if got := l.Name(); got != "slog" {
		t.Errorf("Name() = %q, want slog", got)
	}

	if err := l.Sync(); err != nil {
		t.Errorf("Sync() error = %v, want nil", err)
	}
}

func TestLevels_allBuilders_nonNil(t *testing.T) {
	t.Parallel()

	l := New(log.Options{MinLevel: log.LevelDebug})

	for _, ev := range []log.Event{l.Debug(), l.Info(), l.Warn(), l.Error(), l.Fatal()} {
		if ev == nil {
			t.Error("level builder = nil, want non-nil event")
		}
	}
}

func TestLogger_WithContext(t *testing.T) {
	t.Parallel()

	l, _ := newBuffered(log.LevelDebug)

	if got := l.WithContext(context.Background()); got == nil {
		t.Fatal("WithContext() = nil, want logger")
	}

	ctx := log.ContextWithLogger(context.Background(), l)
	if got := l.WithContext(ctx); got != log.Logger(l) {
		t.Errorf("WithContext(stored) = %v, want stored logger", got)
	}
}

func TestEvent_enabled_emitsJSON(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)
	l.Info().
		Str("s", "v").
		Int("i", 1).
		Int64("i64", 2).
		Float64("f", 1.5).
		Bool("b", true).
		Dur("d", time.Second).
		Time("t", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)).
		Err(errors.New("boom")).
		AnErr("cause", errors.New("root")).
		Any("a", "x").
		Msg("hello")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", got["msg"])
	}

	if got["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got["level"])
	}

	for _, key := range []string{"s", "i", "i64", "f", "b", "d", "t", "error", "cause", "a"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in %v", key, got)
		}
	}
}

func TestEvent_nilErrors_skipped(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)
	l.Info().Err(nil).AnErr("cause", nil).Msg("m")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if _, ok := got["error"]; ok {
		t.Errorf("error key present, want skipped: %v", got)
	}

	if _, ok := got["cause"]; ok {
		t.Errorf("cause key present, want skipped: %v", got)
	}
}

func TestEvent_disabledLevel_doesNotWrite(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelFatal)
	l.Debug().Str("k", "v").Msg("suppressed")
	l.Info().Msgf("suppressed %d", 1)
	l.Debug().Send()

	if buf.Len() != 0 {
		t.Errorf("buffer = %q, want empty", buf.String())
	}
}

func TestEvent_msgf_formats(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)
	l.Info().Msgf("hi %s", "there")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["msg"] != "hi there" {
		t.Errorf("msg = %v, want %q", got["msg"], "hi there")
	}
}

func TestEvent_send_emptyMessage(t *testing.T) {
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

func TestEvent_fatal_rendersErrorPlusFour(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)
	l.Fatal().Msg("fatal")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["level"] != "ERROR+4" {
		t.Errorf("level = %v, want ERROR+4", got["level"])
	}
}

func TestContext_chain_buildsLogger(t *testing.T) {
	t.Parallel()

	l, buf := newBuffered(log.LevelDebug)

	child := l.With().
		Str("base", "v").
		Int("n", 1).
		Int64("n64", 2).
		Float64("f", 1.5).
		Bool("b", true).
		Dur("d", time.Second).
		Time("t", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)).
		Err(errors.New("boom")).
		AnErr("cause", errors.New("root")).
		Any("a", nil).
		Logger()

	child.Info().Msg("child")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v (raw = %q)", err, buf.String())
	}

	if got["base"] != "v" {
		t.Errorf("base = %v, want v", got["base"])
	}

	if got["n"] != float64(1) {
		t.Errorf("n = %v, want 1", got["n"])
	}

	if got["error"] != "boom" {
		t.Errorf("error = %v, want boom", got["error"])
	}
}
