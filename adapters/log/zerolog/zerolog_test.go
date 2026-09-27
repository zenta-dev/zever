package zerolog

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

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

func TestAdapter_identity(t *testing.T) {
	t.Parallel()

	l := New(log.Options{})

	if got := l.Name(); got != "zerolog" {
		t.Errorf("Name() = %q, want zerolog", got)
	}

	if err := l.Sync(); err != nil {
		t.Errorf("Sync() error = %v, want nil", err)
	}
}

func TestEvent_disabledLevel_doesNotWrite(t *testing.T) {
	t.Parallel()

	l := New(log.Options{MinLevel: log.LevelFatal})

	l.Debug().Str("k", "v").Int("i", 1).Bool("b", true).
		Dur("d", time.Second).Time("t", time.Now()).Any("a", nil).Msg("suppressed")
	l.Info().Msgf("suppressed %d", 1)
	l.Debug().Send()
}

func TestNew_unknownMinLevel_fallsBackToInfo(t *testing.T) {
	t.Parallel()

	l := New(log.Options{MinLevel: log.Level(99)})

	if l.Enabled(log.LevelDebug) {
		t.Error("Enabled(debug) = true, want false (unknown min level falls back to info)")
	}

	if !l.Enabled(log.LevelInfo) {
		t.Error("Enabled(info) = false, want true (unknown min level falls back to info)")
	}
}

func TestLevels_allBuilders_nonNil(t *testing.T) {
	t.Parallel()

	// Builder creation alone never emits or exits (exit happens only on Msg/Send),
	// so touching Fatal() here is safe.
	l := New(log.Options{MinLevel: log.LevelDebug})

	for _, ev := range []log.Event{l.Debug(), l.Info(), l.Warn(), l.Error(), l.Fatal()} {
		if ev == nil {
			t.Error("level builder = nil, want non-nil event")
		}
	}
}

func TestEvent_enabledChain_returnsSameEvent(t *testing.T) {
	t.Parallel()

	l := New(log.Options{MinLevel: log.LevelFatal})

	ev := l.Error()
	chained := ev.Str("k", "v").Int("i", 1).Int64("i64", 1).Float64("f", 1.5).
		Bool("b", true).Dur("d", time.Second).Time("t", time.Now()).
		Err(nil).AnErr("e", nil).Any("a", "b")

	if chained == nil {
		t.Fatal("chained event = nil, want non-nil")
	}
}

func TestContext_chain_buildsLogger(t *testing.T) {
	t.Parallel()

	l := New(log.Options{}).With().
		Str("s", "v").Int("i", 1).Int64("i64", 1).Float64("f", 1.5).
		Bool("b", true).Dur("d", time.Second).Time("t", time.Now()).
		Err(nil).AnErr("e", nil).Any("a", nil).Logger()

	if l == nil {
		t.Fatal("Logger() = nil, want logger")
	}

	_ = l.WithContext(t.Context())
}
