package log

import (
	"context"
	"fmt"
	"sync"
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

// Factory creates a Logger from the given options.
type Factory func(opts Options) (Logger, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an adapter with its factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a Logger for a registered adapter using the given options.
func Open(adapter Adapter, opts Options) (Logger, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	logger, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("log: open %s: %w", adapter, err)
	}

	return logger, nil
}
