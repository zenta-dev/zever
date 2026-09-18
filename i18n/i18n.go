package i18n

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// I18n defines the core operations for internationalized message lookup.
type I18n interface {
	// Translate returns the message for locale and key, interpolating args.
	Translate(ctx context.Context, locale, key string, args map[string]string) (string, error)
	// Locales lists the available locales.
	Locales(ctx context.Context) ([]string, error)
	// Close shuts down the backend and releases associated resources.
	Close() error
}

// Factory creates an I18n from the given Options.
type Factory func(opts Options) (I18n, error)

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

// Open creates an I18n for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (I18n, error) {
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	n, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("i18n: open %s: %w", adapter, err)
	}

	return n, nil
}
