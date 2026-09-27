package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/eventbus"
)

var _ eventbus.Pusher = (*bus)(nil)

// subscription is a single handler registration.
type subscription struct {
	ch      chan eventbus.Message
	done    chan struct{}
	dropped atomic.Uint64
}

// Dropped reports how many messages were dropped for this subscription.
func (s *subscription) Dropped() uint64 {
	return s.dropped.Load()
}

type bus struct {
	mu             sync.RWMutex
	topics         map[string][]*subscription
	sem            chan struct{}
	closed         atomic.Bool
	wg             sync.WaitGroup
	buffer         int
	handlerTimeout time.Duration
	closeTimeout   time.Duration
	onPanic        func(topic string, msg eventbus.Message, r any)
}

// New creates an in-memory eventbus from opts.
// The returned bus supports both the push and pull APIs.
func New(opts eventbus.Options) (eventbus.EventBus, error) {
	b, err := newBus(opts)
	if err != nil {
		return nil, err
	}

	return eventbus.Wrap(b), nil
}

func newBus(opts eventbus.Options) (*bus, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	buffer := opts.BufferSize
	if buffer == 0 {
		buffer = eventbus.DefaultBufferSize
	}

	maxHandlers := opts.MaxHandlers
	if maxHandlers == 0 {
		maxHandlers = eventbus.DefaultMaxHandlers
	}

	handlerTimeout := opts.HandlerTimeout
	if handlerTimeout == 0 {
		handlerTimeout = eventbus.DefaultHandlerTimeout
	}

	closeTimeout := opts.CloseTimeout
	if closeTimeout == 0 {
		closeTimeout = eventbus.DefaultCloseTimeout
	}

	return &bus{
		topics:         make(map[string][]*subscription),
		sem:            make(chan struct{}, maxHandlers),
		buffer:         buffer,
		handlerTimeout: handlerTimeout,
		closeTimeout:   closeTimeout,
		onPanic:        opts.OnPanic,
	}, nil
}

// Name returns the adapter name.
func (b *bus) Name() string {
	return "memory"
}

func validateTopic(topic string) error {
	if topic == "" {
		return &eventbus.InvalidOptionsError{Reason: "topic must be non-empty"}
	}

	if len(topic) > eventbus.MaxTopicLen {
		return &eventbus.InvalidOptionsError{Reason: "topic too long"}
	}

	return nil
}

// Publish delivers payload with headers to all subscribers of topic.
func (b *bus) Publish(ctx context.Context, topic string, payload eventbus.Payload, headers eventbus.Headers) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if b.closed.Load() {
		return eventbus.ErrClosed
	}

	if err := validateTopic(topic); err != nil {
		return err
	}

	if len(payload) > eventbus.MaxMessageSize {
		return eventbus.ErrPayloadTooLarge
	}

	base := eventbus.NewMessage(topic, payload, headers)

	b.mu.RLock()
	subs := append([]*subscription(nil), b.topics[topic]...)
	b.mu.RUnlock()

	for _, sub := range subs {
		m := base.Clone()

		select {
		case sub.ch <- m:
		default:
			sub.dropped.Add(1)
		}
	}

	return nil
}

// Subscribe registers handler for topic.
func (b *bus) Subscribe(ctx context.Context, topic string, handler eventbus.Handler) (func(), error) {
	if handler == nil {
		return nil, eventbus.ErrNilHandler
	}

	if b.closed.Load() {
		return nil, eventbus.ErrClosed
	}

	if err := validateTopic(topic); err != nil {
		return nil, err
	}

	sub := &subscription{
		ch:   make(chan eventbus.Message, b.buffer),
		done: make(chan struct{}),
	}

	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return nil, eventbus.ErrClosed
	}

	b.topics[topic] = append(b.topics[topic], sub)
	b.mu.Unlock()

	b.wg.Add(1)

	go b.forward(ctx, topic, sub, handler)

	var once sync.Once

	unsub := func() {
		once.Do(func() {
			b.mu.Lock()

			subs := b.topics[topic]
			for i, s := range subs {
				if s == sub {
					b.topics[topic] = append(subs[:i:i], subs[i+1:]...)
					break
				}
			}

			b.mu.Unlock()

			close(sub.done)

			for {
				select {
				case <-sub.ch:
				default:
					return
				}
			}
		})
	}

	return unsub, nil
}

func (b *bus) forward(parent context.Context, topic string, sub *subscription, handler eventbus.Handler) {
	defer b.wg.Done()
	defer func() {
		for {
			select {
			case <-sub.ch:
			default:
				return
			}
		}
	}()

	for {
		select {
		case <-sub.done:
			return
		case msg := <-sub.ch:

			select {
			case b.sem <- struct{}{}:
			case <-sub.done:
				return
			}

			func(m eventbus.Message) {
				defer func() { <-b.sem }()

				ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), b.handlerTimeout)
				defer cancel()

				doneCh := make(chan struct{})

				go func() {
					defer close(doneCh)

					defer func() {
						if r := recover(); r != nil {
							if b.onPanic != nil {
								b.onPanic(topic, m, r)
							}
						}
					}()

					handler(ctx, m)
				}()

				select {
				case <-doneCh:
				case <-ctx.Done():
				case <-sub.done:
				}
			}(msg)
		}
	}
}

// Close shuts down the eventbus. It is idempotent and always reports nil:
// a close timeout abandons in-flight handlers the same way the redis
// adapter does rather than surfacing DeadlineExceeded.
func (b *bus) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	b.mu.Lock()
	subs := make([]*subscription, 0)
	for _, list := range b.topics {
		subs = append(subs, list...)
	}
	b.topics = make(map[string][]*subscription)
	b.mu.Unlock()

	for _, sub := range subs {
		close(sub.done)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(b.closeTimeout):
	}

	return nil
}
