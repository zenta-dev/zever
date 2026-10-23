package container

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// SIGHUP reload.
//
// Process configuration is restart-only, except the three batteries below:
// on SIGHUP a server re-reads its environment (.env at the project root and
// the process environment) and atomically swaps the log level, the flag
// client, and the rate limiter. Everything else (backends,
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

// With returns a Context for building a child logger. The child follows
// later Swap calls: its fields are re-applied to the new backing logger, so
// a level change reaches loggers derived before the reload.
func (r *ReloadableLogger) With() log.Context { return &childContext{parent: r} }

// WithContext returns a Logger carrying fields stored in ctx, if any. Like
// With, the result follows later Swap calls.
func (r *ReloadableLogger) WithContext(ctx context.Context) log.Logger {
	return (&childLogger{parent: r}).WithContext(ctx)
}

// Enabled reports whether the current logger emits level.
func (r *ReloadableLogger) Enabled(level log.Level) bool { return r.get().Enabled(level) }

// Sync flushes the current logger.
func (r *ReloadableLogger) Sync() error { return r.get().Sync() }

// Name returns the current logger's adapter name.
func (r *ReloadableLogger) Name() string { return r.get().Name() }

// childLogger is a logger derived from a ReloadableLogger. It replays ops
// on the parent's current backing logger and caches the result until the
// parent swaps.
type childLogger struct {
	parent *ReloadableLogger
	ops    []func(log.Logger) log.Logger
	cache  atomic.Pointer[childCache]
}

type childCache struct {
	base   *loggerBox
	logger log.Logger
}

var _ log.Logger = (*childLogger)(nil)

func (c *childLogger) resolve() log.Logger {
	base := c.parent.current.Load()
	if cc := c.cache.Load(); cc != nil && cc.base == base {
		return cc.logger
	}

	l := base.logger
	for _, op := range c.ops {
		l = op(l)
	}

	c.cache.Store(&childCache{base: base, logger: l})

	return l
}

func (c *childLogger) derive(op func(log.Logger) log.Logger) *childLogger {
	ops := make([]func(log.Logger) log.Logger, len(c.ops), len(c.ops)+1)
	copy(ops, c.ops)

	return &childLogger{parent: c.parent, ops: append(ops, op)}
}

func (c *childLogger) Debug() log.Event { return c.resolve().Debug() }
func (c *childLogger) Info() log.Event  { return c.resolve().Info() }
func (c *childLogger) Warn() log.Event  { return c.resolve().Warn() }
func (c *childLogger) Error() log.Event { return c.resolve().Error() }
func (c *childLogger) Fatal() log.Event { return c.resolve().Fatal() }

func (c *childLogger) With() log.Context { return &childContext{parent: c.parent, base: c} }

func (c *childLogger) WithContext(ctx context.Context) log.Logger {
	return c.derive(func(l log.Logger) log.Logger { return l.WithContext(ctx) })
}

func (c *childLogger) Enabled(level log.Level) bool { return c.resolve().Enabled(level) }
func (c *childLogger) Sync() error                  { return c.resolve().Sync() }
func (c *childLogger) Name() string                 { return c.resolve().Name() }

// childContext records field additions so they can be replayed on whichever
// logger is current.
type childContext struct {
	parent *ReloadableLogger
	base   *childLogger
	ops    []func(log.Context) log.Context
}

var _ log.Context = (*childContext)(nil)

func (c *childContext) add(op func(log.Context) log.Context) log.Context {
	ops := make([]func(log.Context) log.Context, len(c.ops), len(c.ops)+1)
	copy(ops, c.ops)

	return &childContext{parent: c.parent, base: c.base, ops: append(ops, op)}
}

func (c *childContext) Str(k, v string) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Str(k, v) })
}

func (c *childContext) Int(k string, v int) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Int(k, v) })
}

func (c *childContext) Int64(k string, v int64) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Int64(k, v) })
}

func (c *childContext) Float64(k string, v float64) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Float64(k, v) })
}

func (c *childContext) Bool(k string, v bool) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Bool(k, v) })
}

func (c *childContext) Dur(k string, v time.Duration) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Dur(k, v) })
}

func (c *childContext) Time(k string, v time.Time) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Time(k, v) })
}

func (c *childContext) Err(err error) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Err(err) })
}

func (c *childContext) AnErr(k string, err error) log.Context {
	return c.add(func(x log.Context) log.Context { return x.AnErr(k, err) })
}

func (c *childContext) Any(k string, v any) log.Context {
	return c.add(func(x log.Context) log.Context { return x.Any(k, v) })
}

func (c *childContext) Logger() log.Logger {
	ops := c.ops
	op := func(l log.Logger) log.Logger {
		x := l.With()
		for _, f := range ops {
			x = f(x)
		}

		return x.Logger()
	}

	if c.base != nil {
		return c.base.derive(op)
	}

	return (&childLogger{parent: c.parent}).derive(op)
}

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

// limiterBox is one backing limiter plus its in-flight accounting. A box
// retired by Swap/Close is closed exactly once, only after every call that
// acquired it has finished.
type limiterBox struct {
	limiter ratelimit.Limiter
	// owned reports whether the ReloadableLimiter closes limiter. The
	// initial limiter is borrowed: its owner (the container) closes it.
	owned bool

	mu       sync.Mutex
	inflight int
	retired  bool

	closeOnce sync.Once
	closeErr  error
}

func (b *limiterBox) acquire() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.retired {
		return false
	}

	b.inflight++

	return true
}

func (b *limiterBox) release() {
	b.mu.Lock()
	b.inflight--
	done := b.retired && b.inflight == 0
	b.mu.Unlock()

	if done {
		b.finish()
	}
}

func (b *limiterBox) retire() {
	b.mu.Lock()
	b.retired = true
	done := b.inflight == 0
	b.mu.Unlock()

	if done {
		b.finish()
	}
}

func (b *limiterBox) finish() {
	if !b.owned {
		return
	}

	b.closeOnce.Do(func() { b.closeErr = b.limiter.Close() })
}

// ReloadableLimiter is a ratelimit.Limiter that atomically swaps its backing
// limiter on SIGHUP. It is goroutine-safe.
//
// Ownership: the initial limiter passed to NewReloadableLimiter is borrowed
// and never closed here (its owner, typically the container, closes it at
// shutdown). Limiters handed to Swap are owned: a replaced one is closed
// once its in-flight Allow/Reset calls drain, and Close closes the current
// one the same way. Callers must not close a limiter passed to a successful
// Swap, nor the one it returns.
type ReloadableLimiter struct {
	mu      sync.Mutex // serializes Swap and Close
	closed  bool
	current atomic.Pointer[limiterBox]
}

var _ ratelimit.Limiter = (*ReloadableLimiter)(nil)

// NewReloadableLimiter builds a ReloadableLimiter fronting initial, which
// must be non-nil. initial stays owned by the caller.
func NewReloadableLimiter(initial ratelimit.Limiter) *ReloadableLimiter {
	r := &ReloadableLimiter{}
	r.current.Store(&limiterBox{limiter: initial})

	return r
}

// Swap replaces the backing limiter with next and reports the replaced
// limiter for information only: the ReloadableLimiter retires it and closes
// it (if it owns it) after in-flight calls drain. A nil next, or a swap
// after Close, is rejected (false) and keeps the current limiter; next is
// then still owned by the caller.
func (r *ReloadableLimiter) Swap(next ratelimit.Limiter) (old ratelimit.Limiter, swapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cur := r.current.Load()
	if next == nil || r.closed {
		return cur.limiter, false
	}

	r.current.Store(&limiterBox{limiter: next, owned: true})
	cur.retire()

	return cur.limiter, true
}

// use runs fn against the current limiter, holding it open until fn returns.
func (r *ReloadableLimiter) use(fn func(ratelimit.Limiter)) {
	for {
		b := r.current.Load()
		if b.acquire() {
			fn(b.limiter)
			b.release()

			return
		}

		// Retired: either replaced (reload current) or the limiter was
		// closed (pass through; the closed limiter reports its own error).
		if r.current.Load() == b {
			fn(b.limiter)

			return
		}
	}
}

// Allow consumes tokens for key on the current limiter.
func (r *ReloadableLimiter) Allow(ctx context.Context, key string, tokens float64) (dec ratelimit.Decision, err error) {
	r.use(func(l ratelimit.Limiter) { dec, err = l.Allow(ctx, key, tokens) })

	return dec, err
}

// Reset clears the bucket state for key on the current limiter.
func (r *ReloadableLimiter) Reset(ctx context.Context, key string) (err error) {
	r.use(func(l ratelimit.Limiter) { err = l.Reset(ctx, key) })

	return err
}

// Close retires the current limiter, closing it once in-flight calls drain
// when this ReloadableLimiter owns it (a borrowed initial limiter is left to
// its owner). Close is idempotent. It returns the close error of an owned
// limiter that had no in-flight calls, else nil.
func (r *ReloadableLimiter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	b := r.current.Load()
	r.closed = true
	b.retire()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inflight == 0 {
		return b.closeErr
	}

	return nil
}

// Name returns the current limiter's adapter name.
func (r *ReloadableLimiter) Name() string { return r.current.Load().limiter.Name() }

// ApplyReload opens the next log and rate-limit state and only then swaps
// both in, so a failure in either leaves every battery exactly as it was.
// limiter and openLimiter may be nil when rate limiting is disabled.
// A replaced logger is synced; a replaced limiter is retired by limiter.
// The returned error is the first open failure, prefixed with the battery.
func ApplyReload(
	logger *ReloadableLogger, openLog func() (log.Logger, error),
	limiter *ReloadableLimiter, openLimiter func() (ratelimit.Limiter, error),
) error {
	nextLog, err := openLog()
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}

	var nextLimiter ratelimit.Limiter

	if limiter != nil && openLimiter != nil {
		nextLimiter, err = openLimiter()
		if err != nil {
			return fmt.Errorf("ratelimit: %w", err)
		}
	}

	if old, swapped := logger.Swap(nextLog); swapped {
		_ = old.Sync()
	}

	if nextLimiter != nil {
		limiter.Swap(nextLimiter)
	}

	return nil
}

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
