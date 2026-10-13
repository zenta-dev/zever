package container

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// SIGHUP reload.
//
// Process configuration is restart-only, except the three batteries below:
// on SIGHUP a server re-reads its config file and atomically swaps the log
// level, the flag client, and the rate limiter. Everything else (adapters,
// DSNs, credentials, TLS material, listen addresses) needs a process
// restart. The swap is all-or-nothing per battery and never blocks request
// handling: readers always see a complete instance, either the old or the
// new one. A failed re-open keeps the old instance and only logs.

// loggerBox is the atomically swapped log.Logger holder.
type loggerBox struct{ logger log.Logger }

// ReloadableLogger is a log.Logger that atomically swaps its backing
// instance on SIGHUP. It is goroutine-safe; Swap reports the replaced
// instance so callers can release it.
type ReloadableLogger struct {
	current atomic.Pointer[loggerBox]
}

var _ log.Logger = (*ReloadableLogger)(nil)

// NewReloadableLogger builds a ReloadableLogger fronting initial, which
// must be non-nil.
func NewReloadableLogger(initial log.Logger) *ReloadableLogger {
	r := &ReloadableLogger{}
	r.current.Store(&loggerBox{logger: initial})

	return r
}

// Swap replaces the backing logger with next and reports the replaced
// instance. A nil next is rejected (false) and keeps the current instance.
func (r *ReloadableLogger) Swap(next log.Logger) (old log.Logger, swapped bool) {
	if next == nil {
		return r.get(), false
	}

	return r.getAndSwap(next), true
}

func (r *ReloadableLogger) get() log.Logger {
	return r.current.Load().logger
}

func (r *ReloadableLogger) getAndSwap(next log.Logger) log.Logger {
	old := r.current.Swap(&loggerBox{logger: next})

	return old.logger
}

// Debug starts a debug-level event on the current logger.
func (r *ReloadableLogger) Debug() log.Event { return r.get().Debug() }

// Info starts an info-level event on the current logger.
func (r *ReloadableLogger) Info() log.Event { return r.get().Info() }

// Warn starts a warn-level event on the current logger.
func (r *ReloadableLogger) Warn() log.Event { return r.get().Warn() }

// Error starts an error-level event on the current logger.
func (r *ReloadableLogger) Error() log.Event { return r.get().Error() }

// Fatal starts a fatal-level event on the current logger.
func (r *ReloadableLogger) Fatal() log.Event { return r.get().Fatal() }

// With returns a Context for building a child logger from the current one.
func (r *ReloadableLogger) With() log.Context { return r.get().With() }

// WithContext returns a Logger carrying fields stored in ctx, if any.
func (r *ReloadableLogger) WithContext(ctx context.Context) log.Logger {
	return r.get().WithContext(ctx)
}

// Enabled reports whether the current logger emits level.
func (r *ReloadableLogger) Enabled(level log.Level) bool { return r.get().Enabled(level) }

// Sync flushes the current logger.
func (r *ReloadableLogger) Sync() error { return r.get().Sync() }

// Name returns the current logger's adapter name.
func (r *ReloadableLogger) Name() string { return r.get().Name() }

// flagBox is the atomically swapped flag.Flag holder.
type flagBox struct{ flags flag.Flag }

// ReloadableFlag is a flag.Flag that atomically swaps its backing client
// on SIGHUP. It is goroutine-safe; Swap reports the replaced client so
// callers can close it.
type ReloadableFlag struct {
	current atomic.Pointer[flagBox]
}

var _ flag.Flag = (*ReloadableFlag)(nil)

// NewReloadableFlag builds a ReloadableFlag fronting initial, which must
// be non-nil.
func NewReloadableFlag(initial flag.Flag) *ReloadableFlag {
	r := &ReloadableFlag{}
	r.current.Store(&flagBox{flags: initial})

	return r
}

// Swap replaces the backing client with next and reports the replaced
// client. A nil next is rejected (false) and keeps the current client.
func (r *ReloadableFlag) Swap(next flag.Flag) (old flag.Flag, swapped bool) {
	if next == nil {
		return r.get(), false
	}

	oldBox := r.current.Swap(&flagBox{flags: next})

	return oldBox.flags, true
}

func (r *ReloadableFlag) get() flag.Flag {
	return r.current.Load().flags
}

// Bool evaluates key as a bool on the current client.
func (r *ReloadableFlag) Bool(ctx context.Context, key string, fallback bool) (bool, error) {
	return r.get().Bool(ctx, key, fallback)
}

// String evaluates key as a string on the current client.
func (r *ReloadableFlag) String(ctx context.Context, key string, fallback string) (string, error) {
	return r.get().String(ctx, key, fallback)
}

// Int evaluates key as an int on the current client.
func (r *ReloadableFlag) Int(ctx context.Context, key string, fallback int) (int, error) {
	return r.get().Int(ctx, key, fallback)
}

// JSON decodes key into out on the current client.
func (r *ReloadableFlag) JSON(ctx context.Context, key string, out any, fallback any) error {
	return r.get().JSON(ctx, key, out, fallback)
}

// Close shuts down the current client.
func (r *ReloadableFlag) Close() error { return r.get().Close() }

// limiterBox is the atomically swapped ratelimit.Limiter holder.
type limiterBox struct{ limiter ratelimit.Limiter }

// ReloadableLimiter is a ratelimit.Limiter that atomically swaps its backing
// limiter on SIGHUP. It is goroutine-safe; Swap reports the replaced
// limiter so callers can close it.
type ReloadableLimiter struct {
	current atomic.Pointer[limiterBox]
}

var _ ratelimit.Limiter = (*ReloadableLimiter)(nil)

// NewReloadableLimiter builds a ReloadableLimiter fronting initial, which
// must be non-nil.
func NewReloadableLimiter(initial ratelimit.Limiter) *ReloadableLimiter {
	r := &ReloadableLimiter{}
	r.current.Store(&limiterBox{limiter: initial})

	return r
}

// Swap replaces the backing limiter with next and reports the replaced
// limiter. A nil next is rejected (false) and keeps the current limiter.
func (r *ReloadableLimiter) Swap(next ratelimit.Limiter) (old ratelimit.Limiter, swapped bool) {
	if next == nil {
		return r.get(), false
	}

	oldBox := r.current.Swap(&limiterBox{limiter: next})

	return oldBox.limiter, true
}

func (r *ReloadableLimiter) get() ratelimit.Limiter {
	return r.current.Load().limiter
}

// Allow consumes tokens for key on the current limiter.
func (r *ReloadableLimiter) Allow(ctx context.Context, key string, tokens float64) (ratelimit.Decision, error) {
	return r.get().Allow(ctx, key, tokens)
}

// Reset clears the bucket state for key on the current limiter.
func (r *ReloadableLimiter) Reset(ctx context.Context, key string) error {
	return r.get().Reset(ctx, key)
}

// Close shuts down the current limiter.
func (r *ReloadableLimiter) Close() error { return r.get().Close() }

// Name returns the current limiter's adapter name.
func (r *ReloadableLimiter) Name() string { return r.get().Name() }

// WatchSIGHUP calls onReload with ctx on every SIGHUP until ctx is done.
// Registration happens synchronously, so a SIGHUP sent after WatchSIGHUP
// returns is always observed. The returned stop unregisters and blocks
// until the watcher exits; cancelling ctx exits it as well. It never
// touches SIGINT/SIGTERM handling.
func WatchSIGHUP(ctx context.Context, onReload func(ctx context.Context)) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)

	done := make(chan struct{})

	go func() {
		defer close(done)
		defer signal.Stop(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				onReload(ctx)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}
