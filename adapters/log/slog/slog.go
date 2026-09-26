package slog

import (
	"context"
	"io"
	"os"

	stdslog "log/slog"

	"github.com/zenta-dev/zever/log"
)

type slogLogger struct {
	logger *stdslog.Logger
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

	handler := stdslog.NewJSONHandler(w, &stdslog.HandlerOptions{
		Level: toSlogLevel(opts.MinLevel),
	})

	return &slogLogger{logger: stdslog.New(handler)}
}

func (l *slogLogger) Debug() log.Event { return l.event(stdslog.LevelDebug) }

func (l *slogLogger) Info() log.Event { return l.event(stdslog.LevelInfo) }

func (l *slogLogger) Warn() log.Event { return l.event(stdslog.LevelWarn) }

func (l *slogLogger) Error() log.Event { return l.event(stdslog.LevelError) }

func (l *slogLogger) Fatal() log.Event { return l.event(fatalLevel) }

func (l *slogLogger) event(level stdslog.Level) log.Event {
	return &slogEvent{logger: l.logger, level: level}
}

func (l *slogLogger) With() log.Context {
	return &slogContext{logger: l.logger}
}

func (l *slogLogger) WithContext(ctx context.Context) log.Logger {
	return log.FromContext(ctx, l)
}

func (l *slogLogger) Enabled(level log.Level) bool {
	return l.logger.Enabled(context.Background(), toSlogLevel(level))
}

func (l *slogLogger) Sync() error { return nil }

func (l *slogLogger) Name() string { return "slog" }
