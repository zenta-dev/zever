package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/shared/registry"
)

// Queue defines the core operations for interacting with a topic-based message queue.
// It handles enqueuing, delivery, and lifecycle management.
type Queue interface {
	// Push enqueues a message on topic with the given payload and headers.
	Push(
		ctx context.Context,
		topic string,
		payload Payload,
		headers Headers,
	) error

	// PushDelayed enqueues a message on topic that becomes available after delay.
	PushDelayed(
		ctx context.Context,
		topic string,
		payload Payload,
		headers Headers,
		delay time.Duration,
	) error

	// Pop dequeues the next available message from topic.
	// It returns ErrEmpty or EmptyError when no message is available.
	Pop(
		ctx context.Context,
		topic string,
	) (Message, error)

	// Ack acknowledges successful processing of msg and removes it from visibility tracking.
	Ack(
		ctx context.Context,
		msg Message,
	) error

	// Nack reports processing failure for msg and optionally requeues it when requeue is true.
	Nack(
		ctx context.Context,
		msg Message,
		requeue bool,
	) error

	// Length returns the number of ready messages in topic.
	Length(
		ctx context.Context,
		topic string,
	) (int64, error)

	// IsEmpty reports whether topic contains no ready messages.
	IsEmpty(ctx context.Context, topic string) (bool, error)

	// Close shuts down the queue and releases associated resources.
	Close() error

	// Name returns the adapter name for the queue.
	Name() string
}

// Factory creates a Queue from the given Options.
type Factory func(opts Options) (Queue, error)

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

// Open creates a Queue for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Queue, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("queue: open %s: %w", adapter, err)
	}

	return c, nil
}
