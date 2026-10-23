package queue

import (
	"context"
	"time"
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
