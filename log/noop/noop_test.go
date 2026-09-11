package noop

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/log"
)

var (
	_ log.Logger  = noopLogger{}
	_ log.Event   = noopEvent{}
	_ log.Context = noopContext{}
)

func TestNew_implementsLogger(t *testing.T) {
	t.Parallel()

	l := New()
	if l == nil {
		t.Fatal("New() = nil, want logger")
	}

	if got := l.Name(); got != "noop" {
		t.Errorf("Name() = %q, want noop", got)
	}
}

func TestLogger_noopBehavior(t *testing.T) {
	t.Parallel()

	l := New()

	if l.Enabled(log.LevelDebug) || l.Enabled(log.LevelFatal) {
		t.Error("Enabled() = true, want false for all levels")
	}

	if err := l.Sync(); err != nil {
		t.Errorf("Sync() error = %v, want nil", err)
	}

	if got := l.WithContext(context.Background()); got != l {
		t.Errorf("WithContext() = %v, want same logger", got)
	}
}

func TestLogger_allLevels_returnEvents(t *testing.T) {
	t.Parallel()

	l := New()

	for _, ev := range []log.Event{l.Debug(), l.Info(), l.Warn(), l.Error(), l.Fatal()} {
		if ev == nil {
			t.Error("level event = nil, want non-nil event")
		}
	}
}

func TestEvent_fullChain_doesNotPanic(t *testing.T) {
	t.Parallel()

	l := New()

	for _, ev := range []log.Event{l.Debug(), l.Info(), l.Warn(), l.Error(), l.Fatal()} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("event chain panicked: %v", r)
				}
			}()

			ev.Str("s", "v").Int("i", -1).Int64("i64", 1).Float64("f", 3.14).
				Bool("b", true).Dur("d", time.Second).Time("t", time.Now()).
				Err(nil).AnErr("e", nil).Any("a", nil).Msg("done")
			ev.Msgf("hello %s", "world")
			ev.Send()
		}()
	}
}

func TestContext_fullChain_returnsLogger(t *testing.T) {
	t.Parallel()

	l := New()
	c := l.With().
		Str("s", "v").Int("i", 1).Int64("i64", 1).Float64("f", 1.5).
		Bool("b", true).Dur("d", time.Second).Time("t", time.Now()).
		Err(nil).AnErr("e", nil).Any("a", nil)

	if c.Logger() == nil {
		t.Fatal("Logger() = nil, want logger")
	}
}
