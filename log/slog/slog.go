package slog

import (
	"context"
	"os"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

type slogLogger struct {
	logger *stdslog.Logger
}

// New creates a Logger writing JSON events to stdout at the configured minimum level.
func New(opts log.Options) log.Logger {
	handler := stdslog.NewJSONHandler(os.Stdout, &stdslog.HandlerOptions{
		Level: toSlogLevel(opts.MinLevel),
	})

	return &slogLogger{logger: stdslog.New(handler)}
}

func (a *slogLogger) Debug() log.Event { return a.event(stdslog.LevelDebug) }

func (a *slogLogger) Info() log.Event { return a.event(stdslog.LevelInfo) }

func (a *slogLogger) Warn() log.Event { return a.event(stdslog.LevelWarn) }

func (a *slogLogger) Error() log.Event { return a.event(stdslog.LevelError) }

func (a *slogLogger) Fatal() log.Event { return a.event(fatalLevel) }

func (a *slogLogger) event(level stdslog.Level) log.Event {
	return &slogEvent{logger: a.logger, level: level}
}

func (a *slogLogger) With() log.Context {
	return &slogContext{logger: a.logger}
}

func (a *slogLogger) WithContext(ctx context.Context) log.Logger {
	return log.FromContext(ctx, a)
}

func (a *slogLogger) Enabled(level log.Level) bool {
	return a.logger.Enabled(context.Background(), toSlogLevel(level))
}

func (a *slogLogger) Sync() error { return nil }

func (a *slogLogger) Name() string { return "slog" }
