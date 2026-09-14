package eventbus

import (
	"context"
	"fmt"
	"sync"
)

// Handler processes a message delivered on a subscribed topic.
type Handler func(ctx context.Context, msg Message)

// Eventbus defines the core operations for publishing and subscribing to topics.
type Eventbus interface {
	// Publish delivers payload with headers to all subscribers of topic.
	Publish(
		ctx context.Context,
		topic string,
		payload Payload,
		headers Headers,
	) error

	// Subscribe registers handler for topic.
	// It returns an unsubscribe function that detaches the handler.
	Subscribe(
		ctx context.Context,
		topic string,
		handler Handler,
	) (unsubscribe func(), err error)

	// Close shuts down the eventbus and releases associated resources.
	Close() error

	// Name returns the adapter name for the eventbus.
	Name() string
}

// Factory creates an Eventbus from the given Options.
type Factory func(opts Options) (Eventbus, error)

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

// Open creates an Eventbus for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Eventbus, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	b, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("eventbus: open %s: %w", adapter, err)
	}

	return b, nil
}
