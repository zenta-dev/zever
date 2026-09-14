package mailer

import (
	"context"
	"fmt"
	"sync"
)

// Mailer defines the core operations for sending mail.
type Mailer interface {
	// Send delivers msg.
	Send(ctx context.Context, msg *Mail) error
	// Close shuts down the mailer and releases associated resources.
	Close() error
}

// Factory creates a Mailer from the given Options.
type Factory func(opts Options) (Mailer, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
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

// Open creates a Mailer for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Mailer, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	m, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("mailer: open %s: %w", adapter, err)
	}

	return m, nil
}
