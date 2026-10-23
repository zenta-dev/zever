package eventbus

import (
	"context"
)

// Handler processes a message delivered on a subscribed topic.
type Handler func(ctx context.Context, msg Message)

// Pusher is the push-only transport core implemented by adapters.
// Use Wrap (or Open, which wraps automatically) to upgrade a Pusher to a
// full EventBus with pull support.
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

// validateTopic checks a topic name: non-empty, at most MaxTopicLen.
func validateTopic(topic string) error {
	if topic == "" {
		return InvalidOptionsError{Reason: "topic must be non-empty"}
	}

	if len(topic) > MaxTopicLen {
		return InvalidOptionsError{Reason: "topic too long"}
	}

	return nil
}
