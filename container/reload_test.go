package container

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// stubEvent is a no-op log.Event for reload tests.
type stubEvent struct{}

func (stubEvent) Str(string, string) log.Event        { return stubEvent{} }
func (stubEvent) Int(string, int) log.Event           { return stubEvent{} }
func (stubEvent) Int64(string, int64) log.Event       { return stubEvent{} }
func (stubEvent) Float64(string, float64) log.Event   { return stubEvent{} }
func (stubEvent) Bool(string, bool) log.Event         { return stubEvent{} }
func (stubEvent) Dur(string, time.Duration) log.Event { return stubEvent{} }
func (stubEvent) Time(string, time.Time) log.Event    { return stubEvent{} }
func (stubEvent) Err(error) log.Event                 { return stubEvent{} }
func (stubEvent) AnErr(string, error) log.Event       { return stubEvent{} }
func (stubEvent) Any(string, any) log.Event           { return stubEvent{} }
func (stubEvent) Msg(string)                          {}
func (stubEvent) Msgf(string, ...any)                 {}
func (stubEvent) Send()                               {}

// stubContext is a no-op log.Context for reload tests.
type stubContext struct{ l log.Logger }

func (c *stubContext) Str(string, string) log.Context      { return c }
func (c *stubContext) Int(string, int) log.Context         { return c }
func (c *stubContext) Int64(string, int64) log.Context     { return c }
func (c *stubContext) Float64(string, float64) log.Context { return c }
func (c *stubContext) Bool(string, bool) log.Context       { return c }
func (c *stubContext) Dur(string, time.Duration) log.Context {
	return c
}
func (c *stubContext) Time(string, time.Time) log.Context { return c }
func (c *stubContext) Err(error) log.Context              { return c }
func (c *stubContext) AnErr(string, error) log.Context    { return c }
func (c *stubContext) Any(string, any) log.Context        { return c }
func (c *stubContext) Logger() log.Logger                 { return c.l }

// stubLogger is a log.Logger with a fixed minimum level for reload tests.
type stubLogger struct{ min log.Level }

var _ log.Logger = (*stubLogger)(nil)

func (l *stubLogger) Debug() log.Event { return stubEvent{} }
func (l *stubLogger) Info() log.Event  { return stubEvent{} }
func (l *stubLogger) Warn() log.Event  { return stubEvent{} }
func (l *stubLogger) Error() log.Event { return stubEvent{} }
func (l *stubLogger) Fatal() log.Event { return stubEvent{} }
func (l *stubLogger) With() log.Context {
	return &stubContext{l: l}
}
func (l *stubLogger) WithContext(context.Context) log.Logger { return l }
func (l *stubLogger) Enabled(level log.Level) bool           { return level >= l.min }
func (l *stubLogger) Sync() error                            { return nil }
func (l *stubLogger) Name() string                           { return "stub" }

// stubFlag is a flag.Flag backed by a fixed map for reload tests.
type stubFlag struct{ flags map[string]any }

var _ flag.Flag = (*stubFlag)(nil)

func (f *stubFlag) boolOf(key string) (any, bool) {
	v, ok := f.flags[key]

	return v, ok
}

func (f *stubFlag) Bool(_ context.Context, key string, fallback bool) (bool, error) {
	if v, ok := f.boolOf(key); ok {
		if b, is := v.(bool); is {
			return b, nil
		}
	}

	return fallback, nil
}

func (f *stubFlag) String(_ context.Context, key string, fallback string) (string, error) {
	if v, ok := f.boolOf(key); ok {
		if s, is := v.(string); is {
			return s, nil
		}
	}

	return fallback, nil
}

func (f *stubFlag) Int(_ context.Context, _ string, fallback int) (int, error) {
	return fallback, nil
}

func (f *stubFlag) JSON(_ context.Context, _ string, _ any, _ any) error {
	return nil
}

func (f *stubFlag) Close() error { return nil }

// stubLimiter is a ratelimit.Limiter with a fixed verdict for reload tests.
type stubLimiter struct{ allowed bool }

var _ ratelimit.Limiter = (*stubLimiter)(nil)

func (l *stubLimiter) Allow(context.Context, string, float64) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: l.allowed}, nil
}

func (l *stubLimiter) Reset(context.Context, string) error { return nil }
func (l *stubLimiter) Close() error                        { return nil }
func (l *stubLimiter) Name() string                        { return "stub" }

// sighupSelf delivers a real SIGHUP to this process. Callers must hold a
// WatchSIGHUP registration so delivery lands on a channel instead of the
// default kill action. Not parallel-safe with other signal tests.
func sighupSelf(t *testing.T) {
	t.Helper()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("signal SIGHUP: %v", err)
	}
}

func TestReloadableLoggerSwapAppliesLevel(t *testing.T) {
	r := NewReloadableLogger(&stubLogger{min: log.LevelInfo})

	if r.Enabled(log.LevelDebug) {
		t.Fatal("debug enabled before swap")
	}

	old, swapped := r.Swap(&stubLogger{min: log.LevelDebug})
	if !swapped {
		t.Fatal("Swap returned false")
	}

	if old == nil {
		t.Fatal("Swap returned nil old logger")
	}

	if !r.Enabled(log.LevelDebug) {
		t.Error("debug not enabled after swap")
	}
}

func TestReloadableLoggerSwapNilKeepsOld(t *testing.T) {
	r := NewReloadableLogger(&stubLogger{min: log.LevelInfo})

	if _, swapped := r.Swap(nil); swapped {
		t.Fatal("Swap(nil) returned true")
	}

	if r.Enabled(log.LevelDebug) {
		t.Error("nil swap changed the level")
	}
}

func TestReloadableFlagSwapAppliesValue(t *testing.T) {
	r := NewReloadableFlag(&stubFlag{flags: map[string]any{"dark": false}})

	got, err := r.Bool(t.Context(), "dark", true)
	if err != nil || got {
		t.Fatalf("Bool = %v, %v before swap, want false, nil", got, err)
	}

	if _, swapped := r.Swap(&stubFlag{flags: map[string]any{"dark": true}}); !swapped {
		t.Fatal("Swap returned false")
	}

	got, err = r.Bool(t.Context(), "dark", false)
	if err != nil || !got {
		t.Errorf("Bool = %v, %v after swap, want true, nil", got, err)
	}
}

func TestReloadableLimiterSwapAppliesVerdict(t *testing.T) {
	r := NewReloadableLimiter(&stubLimiter{allowed: false})

	dec, err := r.Allow(t.Context(), "k", 1)
	if err != nil || dec.Allowed {
		t.Fatalf("Allow = %+v, %v before swap, want denied, nil", dec, err)
	}

	if _, swapped := r.Swap(&stubLimiter{allowed: true}); !swapped {
		t.Fatal("Swap returned false")
	}

	dec, err = r.Allow(t.Context(), "k", 1)
	if err != nil || !dec.Allowed {
		t.Errorf("Allow = %+v, %v after swap, want allowed, nil", dec, err)
	}
}

func TestWatchSIGHUPSwapsLogLevel(t *testing.T) {
	r := NewReloadableLogger(&stubLogger{min: log.LevelInfo})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reloaded := make(chan struct{})

	stop := WatchSIGHUP(ctx, func(context.Context) {
		r.Swap(&stubLogger{min: log.LevelDebug})

		select {
		case <-reloaded:
		default:
			close(reloaded)
		}
	})
	defer stop()

	sighupSelf(t)

	timeout, stopTimeout := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopTimeout()

	select {
	case <-reloaded:
	case <-timeout.Done():
		t.Fatal("SIGHUP reload did not fire")
	}

	if !r.Enabled(log.LevelDebug) {
		t.Error("new log level does not apply after SIGHUP")
	}
}

func TestWatchSIGHUPSwapsFlagValue(t *testing.T) {
	r := NewReloadableFlag(&stubFlag{flags: map[string]any{"dark": false}})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reloaded := make(chan struct{})

	stop := WatchSIGHUP(ctx, func(context.Context) {
		r.Swap(&stubFlag{flags: map[string]any{"dark": true}})

		select {
		case <-reloaded:
		default:
			close(reloaded)
		}
	})
	defer stop()

	sighupSelf(t)

	timeout, stopTimeout := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopTimeout()

	select {
	case <-reloaded:
	case <-timeout.Done():
		t.Fatal("SIGHUP reload did not fire")
	}

	got, err := r.Bool(t.Context(), "dark", false)
	if err != nil || !got {
		t.Errorf("Bool = %v, %v after SIGHUP, want true, nil", got, err)
	}
}

func TestWatchSIGHUPSwapsRateLimit(t *testing.T) {
	r := NewReloadableLimiter(&stubLimiter{allowed: false})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var calls atomic.Int32

	reloaded := make(chan struct{})

	stop := WatchSIGHUP(ctx, func(context.Context) {
		r.Swap(&stubLimiter{allowed: true})

		if calls.Add(1) == 1 {
			close(reloaded)
		}
	})
	defer stop()

	sighupSelf(t)

	timeout, stopTimeout := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopTimeout()

	select {
	case <-reloaded:
	case <-timeout.Done():
		t.Fatal("SIGHUP reload did not fire")
	}

	dec, err := r.Allow(t.Context(), "k", 1)
	if err != nil || !dec.Allowed {
		t.Errorf("Allow = %+v, %v after SIGHUP, want allowed, nil", dec, err)
	}
}

func TestWatchSIGHUPStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var calls atomic.Int32

	stop := WatchSIGHUP(ctx, func(context.Context) { calls.Add(1) })

	cancel()
	stop()

	// A probe listener absorbs the signal so the process survives: its
	// receipt proves delivery happened while the watcher was stopped,
	// which makes the zero-reload assertion deterministic (no sleep).
	probe := make(chan os.Signal, 1)
	signal.Notify(probe, syscall.SIGHUP)
	defer signal.Stop(probe)

	sighupSelf(t)

	timeout, stopTimeout := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopTimeout()

	select {
	case <-probe:
	case <-timeout.Done():
		t.Fatal("SIGHUP was not delivered")
	}

	if got := calls.Load(); got != 0 {
		t.Errorf("reloads after stop = %d, want 0", got)
	}
}
