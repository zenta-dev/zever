// Package logtest provides the conformance kit third-party log adapters run to prove backend parity.
package logtest

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

// Conformance verifies factory-built loggers implement the log.Logger
// contract: open/register round-trip, every level emits with every
// field type without panicking, With/Context chaining preserves the
// logger, Enabled/Sync/Name behave, and ParseLevel sentinels hold.
// Each subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network.
//
// Cleanup is Sync, not Close: the Logger interface has no Close
// method, so the kit asserts Sync returns nil instead.
// Documented no-op exemption: the noop adapter discards everything
// and reports Enabled=false for all levels; the kit accepts either
// Enabled value and only requires emission not to panic.
func Conformance(t *testing.T, factory func(t *testing.T) log.Logger) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("EmitLevels", func(t *testing.T) { conformanceEmitLevels(t, factory) })
	t.Run("WithChaining", func(t *testing.T) { conformanceWithChaining(t, factory) })
	t.Run("Context", func(t *testing.T) { conformanceContext(t, factory) })
	t.Run("ParseLevel", func(t *testing.T) { conformanceParseLevel(t) })
	t.Run("Sync", func(t *testing.T) { conformanceSync(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := log.Open(log.Adapter("conformance-missing-adapter"), log.Options{}); !errors.Is(err, log.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := log.Adapter("conformance-probe-log")

	if err := log.Register(probe, nil); !errors.Is(err, log.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(log.Options) (log.Logger, error) {
		return nil, errors.New("logtest: probe factory must not run")
	}

	_ = log.Register(probe, stub)

	if err := log.Register(probe, stub); !errors.Is(err, log.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceEmitLevels(t *testing.T, factory func(t *testing.T) log.Logger) {
	t.Helper()

	l := factory(t)

	if l.Name() == "" {
		t.Error("Name() is empty")
	}

	// Enabled may be true or false per adapter (noop is always
	// false); the call itself must not panic. Reference the result
	// so the check is not dead code.
	_ = l.Enabled(log.LevelInfo)

	emit := func(e log.Event) {
		e.Str("s", "v").
			Int("i", -1).
			Int64("i64", 1<<40).
			Float64("f", 1.5).
			Bool("b", true).
			Dur("d", time.Second).
			Time("t", time.Now()).
			Err(errors.New("kit-error")).
			AnErr("custom", errors.New("kit-custom")).
			Any("any", map[string]int{"n": 1}).
			Msg("kit message")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("level emission panicked: %v", r)
			}
		}()

		emit(l.Debug())
		emit(l.Info())
		emit(l.Warn())
		emit(l.Error())
		l.Info().Msgf("kit %s %d", "formatted", 1)
		l.Info().Send()
	}()
}

func conformanceWithChaining(t *testing.T, factory func(t *testing.T) log.Logger) {
	t.Helper()

	l := factory(t)

	child := l.With().Str("req", "1").Int("n", 2).Logger()
	if child == nil {
		t.Fatal("With().Logger() = nil")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("child emission panicked: %v", r)
			}
		}()

		child.Info().Str("k", "v").Msg("child message")
	}()
}

func conformanceContext(t *testing.T, factory func(t *testing.T) log.Logger) {
	t.Helper()

	ctx := t.Context()
	l := factory(t)

	carried := l.WithContext(ctx)
	if carried == nil {
		t.Fatal("WithContext() = nil")
	}

	stored := log.ContextWithLogger(ctx, l)
	if log.FromContext(stored, nil) == nil {
		t.Error("FromContext() = nil, want stored logger")
	}

	if log.FromContext(ctx, l) != l {
		t.Error("FromContext(missing) != fallback, want fallback logger")
	}
}

func conformanceParseLevel(t *testing.T) {
	t.Helper()

	for _, tc := range []struct {
		name string
		want log.Level
	}{
		{"debug", log.LevelDebug},
		{"info", log.LevelInfo},
		{"warn", log.LevelWarn},
		{"error", log.LevelError},
		{"fatal", log.LevelFatal},
	} {
		got, err := log.ParseLevel(tc.name)
		if err != nil || got != tc.want {
			t.Errorf("ParseLevel(%q) = %v,%v want %v,nil", tc.name, got, err, tc.want)
		}
	}

	if _, err := log.ParseLevel("nope"); !errors.Is(err, log.ErrInvalidLevel) {
		t.Errorf("ParseLevel(bad) err = %v, want ErrInvalidLevel", err)
	}
}

func conformanceSync(t *testing.T, factory func(t *testing.T) log.Logger) {
	t.Helper()

	l := factory(t)

	l.Info().Msg("before sync")

	if err := l.Sync(); err != nil {
		t.Errorf("Sync() error = %v, want nil", err)
	}
}
