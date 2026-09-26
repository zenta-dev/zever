package mailer

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
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

// Open creates a Mailer for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Mailer, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	m, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("mailer: open %s: %w", adapter, err)
	}

	return m, nil
}
