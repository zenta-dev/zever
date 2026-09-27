package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// DefaultChanBuffer is the pull-channel buffer applied when SubscribeChan
// is called with a non-positive buffer size.
const DefaultChanBuffer = 256

// chanSub is a single pull subscription. Sends hold mu, so Unsubscribe and
// Close can mark closed under the same mutex before closing ch: close never
// races with a send.
type chanSub struct {
	mu     sync.Mutex
	ch     chan Message
	closed bool
}

type pullEntry struct {
	topic string
	sub   *chanSub
	unsub func()
}

// pullBus upgrades a push-only Pusher with a generic pull API.
// The pull forwarder reuses the push Subscribe path, so adapters stay
// push-only internally.
type pullBus struct {
	inner  Pusher
	mu     sync.Mutex
	subs   map[<-chan Message]*pullEntry
	closed atomic.Bool
}

// Wrap upgrades inner to a full EventBus with pull support.
func Wrap(inner Pusher) EventBus {
	return &pullBus{
		inner: inner,
		subs:  make(map[<-chan Message]*pullEntry),
	}
}

// Publish delivers payload with headers to all subscribers of topic.
func (b *pullBus) Publish(ctx context.Context, topic string, payload Payload, headers Headers) error {
	return b.inner.Publish(ctx, topic, payload, headers)
}

// Subscribe registers handler for topic and returns an unsubscribe function.
func (b *pullBus) Subscribe(ctx context.Context, topic string, handler Handler) (func(), error) {
	return b.inner.Subscribe(ctx, topic, handler)
}

// Name returns the inner adapter name.
func (b *pullBus) Name() string { return b.inner.Name() }

// SubscribeChan registers a buffered pull channel for topic.
func (b *pullBus) SubscribeChan(ctx context.Context, topic string, buffer int) (<-chan Message, error) {
	if b.closed.Load() {
		return nil, ErrClosed
	}

	if err := validateTopic(topic); err != nil {
		return nil, err
	}

	if buffer <= 0 {
		buffer = DefaultChanBuffer
	}

	sub := &chanSub{ch: make(chan Message, buffer)}

	unsub, err := b.inner.Subscribe(ctx, topic, func(_ context.Context, m Message) {
		sub.mu.Lock()
		defer sub.mu.Unlock()

		if sub.closed {
			return
		}

		select {
		case sub.ch <- m:
		default:
			// Full pull channel: drop the newest message for this
			// subscriber only; documented, no counter.
		}
	})
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.Load() {
		sub.mu.Lock()
		sub.closed = true
		sub.mu.Unlock()
		unsub()
		close(sub.ch)

		return nil, ErrClosed
	}

	b.subs[sub.ch] = &pullEntry{topic: topic, sub: sub, unsub: unsub}

	return sub.ch, nil
}

// Unsubscribe detaches a pull channel created by SubscribeChan and closes it.
func (b *pullBus) Unsubscribe(topic string, ch <-chan Message) error {
	if b.closed.Load() {
		return ErrClosed
	}

	b.mu.Lock()
	entry, ok := b.subs[ch]
	if !ok || entry.topic != topic {
		b.mu.Unlock()

		return fmt.Errorf("eventbus: unsubscribe %q: %w", topic, ErrNotSubscribed)
	}

	delete(b.subs, ch)
	b.mu.Unlock()

	entry.sub.mu.Lock()
	entry.sub.closed = true
	entry.sub.mu.Unlock()
	entry.unsub()
	close(entry.sub.ch)

	return nil
}

// Close detaches every pull subscription (closing each channel exactly once),
// then closes the inner pusher. It is idempotent and reports the inner
// Close result.
func (b *pullBus) Close() error {
	if b.closed.CompareAndSwap(false, true) {
		b.mu.Lock()
		subs := b.subs
		b.subs = make(map[<-chan Message]*pullEntry)
		b.mu.Unlock()

		for _, entry := range subs {
			entry.sub.mu.Lock()
			entry.sub.closed = true
			entry.sub.mu.Unlock()
			entry.unsub()
			close(entry.sub.ch)
		}
	}

	return b.inner.Close()
}
