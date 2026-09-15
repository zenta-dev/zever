package i18n

import (
	"context"
	"fmt"
	"sync"
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

// Open creates an I18n for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (I18n, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	n, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("i18n: open %s: %w", adapter, err)
	}

	return n, nil
}
