package zerolog

import (
	"context"
	"io"
	"os"

	zl "github.com/rs/zerolog"

	"github.com/zenta-dev/zever/log"
)

type zerologAdapter struct {
	log zl.Logger
}

// New creates a Logger writing JSON events to stdout at the configured minimum level.
func New(opts log.Options) log.Logger {
	return NewWithWriter(opts, os.Stdout)
}

// NewWithWriter creates a Logger writing JSON events to w at the configured
// minimum level. A nil w falls back to stdout.
func NewWithWriter(opts log.Options, w io.Writer) log.Logger {
	if w == nil {
		w = os.Stdout
	}

	l := zl.New(w).With().Timestamp().Logger().Level(toZeroLogLevel(opts.MinLevel))

	return &zerologAdapter{log: l}
}

func (a *zerologAdapter) Debug() log.Event {
	return wrapZerologEvent(a.log.Debug()) //nolint:zerologlint // wrapped event is dispatched via Msg/Send by the log.Event caller, not here
}

func (a *zerologAdapter) Info() log.Event {
	return wrapZerologEvent(a.log.Info()) //nolint:zerologlint // wrapped event is dispatched via Msg/Send by the log.Event caller, not here
}

func (a *zerologAdapter) Warn() log.Event {
	return wrapZerologEvent(a.log.Warn()) //nolint:zerologlint // wrapped event is dispatched via Msg/Send by the log.Event caller, not here
}

func (a *zerologAdapter) Error() log.Event {
	return wrapZerologEvent(a.log.Error()) //nolint:zerologlint // wrapped event is dispatched via Msg/Send by the log.Event caller, not here
}

func (a *zerologAdapter) Fatal() log.Event {
	return wrapZerologEvent(a.log.Fatal()) //nolint:zerologlint // wrapped event is dispatched via Msg/Send by the log.Event caller, not here
}

func (a *zerologAdapter) With() log.Context {
	return &zerologContext{ctx: a.log.With()}
}

func (a *zerologAdapter) WithContext(ctx context.Context) log.Logger {
	return &zerologAdapter{log: *zl.Ctx(ctx)}
}

func (a *zerologAdapter) Enabled(level log.Level) bool {
	return a.log.GetLevel() <= toZeroLogLevel(level)
}

func (a *zerologAdapter) Sync() error { return nil }

func (a *zerologAdapter) Name() string { return "zerolog" }
