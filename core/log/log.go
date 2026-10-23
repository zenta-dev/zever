package log

import (
	"context"
)

// Logger is the structured logging facade used throughout zever.
type Logger interface {
	// Debug starts a debug-level event.
	Debug() Event
	// Info starts an info-level event.
	Info() Event
	// Warn starts a warn-level event.
	Warn() Event
	// Error starts an error-level event.
	Error() Event
	// Fatal starts a fatal-level event.
	Fatal() Event

	// With returns a Context for building a child logger with additional fields.
	With() Context
	// WithContext returns a Logger carrying fields stored in ctx, if any.
	WithContext(ctx context.Context) Logger

	// Enabled reports whether the given level will be emitted.
	Enabled(level Level) bool
	// Sync flushes any buffered output.
	Sync() error

	// Name returns the adapter name backing this logger.
	Name() string
}

// Factory creates a Logger from the given Options.
type Factory func(opts Options) (Logger, error)
