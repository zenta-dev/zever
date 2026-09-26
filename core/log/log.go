package log

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
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

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Logger for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Logger, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	logger, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("log: open %s: %w", adapter, err)
	}

	return logger, nil
}
