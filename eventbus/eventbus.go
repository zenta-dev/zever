package eventbus

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/registry"
)

// Handler processes a message delivered on a subscribed topic.
type Handler func(ctx context.Context, msg Message)

// Pusher is the push-only transport core implemented by adapters.
// Use Wrap (or Open, which wraps automatically) to upgrade a Pusher to a
// full Eventbus with pull support.
type Pusher interface {
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

	// Close shuts down the pusher and releases associated resources.
	// Instant-close: no context, per the close-shape standard (see
	// container/README.md). Pool-backed services use Close(ctx); the
	// container probes both via closeAny.
	Close() error

	// Name returns the adapter name for the pusher.
	Name() string
}

// EventBus defines the unified operations for publishing and subscribing to
// topics, push (Subscribe) and pull (SubscribeChan) alike.
type EventBus interface {
	// Pusher is the push core promoted into EventBus.
	Pusher

	// SubscribeChan registers a buffered pull channel for topic.
	// A non-positive buffer selects DefaultChanBuffer. When the channel is
	// full the newest message is dropped for that subscriber only; other
	// subscribers are unaffected.
	SubscribeChan(ctx context.Context, topic string, buffer int) (<-chan Message, error)

	// Unsubscribe detaches a pull channel created by SubscribeChan and
	// closes it. An unknown topic returns ErrNotSubscribed.
	Unsubscribe(topic string, ch <-chan Message) error
}

// Eventbus aliases EventBus for compatibility.
type Eventbus = EventBus

// Factory creates an EventBus from the given Options.
type Factory func(opts Options) (EventBus, error)

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

// Open creates an Eventbus for adapter using the registered Factory and opts.
// The returned bus always supports the pull API: factory output is wrapped
// unless it already carries pull support.
func Open(adapter Adapter, opts Options) (Eventbus, error) {
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	b, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("eventbus: open %s: %w", adapter, err)
	}

	if _, ok := b.(*pullBus); ok {
		return b, nil
	}

	return Wrap(b), nil
}

// validateTopic checks a topic name: non-empty, at most MaxTopicLen.
func validateTopic(topic string) error {
	if topic == "" {
		return &InvalidOptionsError{Reason: "topic must be non-empty"}
	}

	if len(topic) > MaxTopicLen {
		return &InvalidOptionsError{Reason: "topic too long"}
	}

	return nil
}
