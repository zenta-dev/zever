package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zenta-dev/zever/queue"
)

type inflight struct {
	msg      queue.Message
	deadline time.Time
}
type memoryAdapter struct {
	mu                sync.RWMutex
	topics            map[string]*topicQueue
	visibilityTimeout time.Duration
	pollTimeout       time.Duration
	buffer            int
	closed            bool
}

// New creates an in-memory adapter with delayed delivery and visibility timeout, defaulting VisibilityTimeout to 30s, PollTimeout to 5s, and Buffer to 10000.
func New(opts queue.Options) (queue.Queue, error) {
	visibilityTimeout := opts.VisibilityTimeout
	if visibilityTimeout <= 0 {
		visibilityTimeout = 30 * time.Second
	}

	pollTimeout := opts.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = 5 * time.Second
	}

	buffer := opts.Buffer
	if buffer <= 0 {
		buffer = 10000
	}

	return &memoryAdapter{
		topics:            make(map[string]*topicQueue),
		visibilityTimeout: visibilityTimeout,
		pollTimeout:       pollTimeout,
		buffer:            buffer,
	}, nil
}

func (a *memoryAdapter) Push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	t, err := a.topic(topic)
	if err != nil {
		return err
	}

	if a.buffer > 0 {
		if err := a.waitForSpace(ctx, t); err != nil {
			return err
		}
	}

	msg := queue.NewMessage(topic, payload, headers)

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return queue.ErrClosed
	}

	t = a.mountTopicLocked(topic, t)
	t.mu.Lock()
	t.ready = append(t.ready, msg)
	t.mu.Unlock()
	a.mu.Unlock()

	t.signal()

	return nil
}

func (a *memoryAdapter) PushDelayed(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	t, err := a.topic(topic)
	if err != nil {
		return err
	}

	if a.buffer > 0 {
		if delay <= 0 {
			if err := a.waitForSpace(ctx, t); err != nil {
				return err
			}
		} else if err := a.waitForDelayedSpace(ctx, t); err != nil {
			return err
		}
	}

	msg := queue.NewMessage(topic, payload, headers)

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return queue.ErrClosed
	}

	t = a.mountTopicLocked(topic, t)
	t.mu.Lock()

	if delay <= 0 {
		t.ready = append(t.ready, msg)
	} else {
		t.delayed = append(t.delayed, delayedEntry{
			msg:     msg,
			readyAt: time.Now().Add(delay),
		})
		t.startPromoterLocked()
	}

	t.mu.Unlock()
	a.mu.Unlock()

	if delay <= 0 {
		t.signal()
	} else {
		select {
		case t.promoteCh <- struct{}{}:
		default:
		}
	}

	return nil
}

func (a *memoryAdapter) Pop(ctx context.Context, topic string) (queue.Message, error) {
	t := a.getTopic(topic)

	if t == nil {
		if err := ctx.Err(); err != nil {
			return queue.Message{}, fmt.Errorf("queue: Pop cancelled: %w", err)
		}

		return queue.Message{}, queue.ErrEmpty
	}

	return a.popWithTopic(ctx, topic, t)
}

func (a *memoryAdapter) Ack(_ context.Context, msg queue.Message) error {
	a.mu.RLock()
	closed := a.closed
	t := a.topics[msg.Topic]
	a.mu.RUnlock()

	if t == nil {
		if closed {
			return queue.ErrClosed
		}

		return nil
	}

	t.mu.Lock()
	if inf, ok := t.inflight[msg.ID]; ok && inf.msg.Attempt == msg.Attempt {
		delete(t.inflight, msg.ID)
	}
	t.mu.Unlock()

	a.sweepTopic(msg.Topic, t)

	return nil
}

func (a *memoryAdapter) Nack(_ context.Context, msg queue.Message, requeue bool) error {
	a.mu.RLock()
	closed := a.closed
	t := a.topics[msg.Topic]
	a.mu.RUnlock()

	if t == nil {
		if closed {
			return queue.ErrClosed
		}

		return nil
	}

	t.mu.Lock()

	inf, ok := t.inflight[msg.ID]
	if ok && inf.msg.Attempt == msg.Attempt {
		delete(t.inflight, msg.ID)

		if requeue {
			inf.msg.Attempt++
			inf.msg = inf.msg.Clone()
			t.ready = append(t.ready, inf.msg)
		}
	}
	t.mu.Unlock()

	a.sweepTopic(msg.Topic, t)

	if ok && requeue {
		t.signal()
	}

	return nil
}

func (a *memoryAdapter) Length(_ context.Context, topic string) (int64, error) {
	t := a.getTopic(topic)
	if t == nil {
		return 0, nil
	}

	t.mu.Lock()
	n := int64(len(t.ready))
	t.mu.Unlock()

	a.sweepTopic(topic, t)

	return n, nil
}

func (a *memoryAdapter) IsEmpty(ctx context.Context, topic string) (bool, error) {
	n, err := a.Length(ctx, topic)

	return n == 0, err
}

func (a *memoryAdapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()

		return nil
	}

	a.closed = true

	topics := make([]*topicQueue, 0, len(a.topics))
	for _, tq := range a.topics {
		topics = append(topics, tq)
	}
	a.mu.Unlock()

	for _, tq := range topics {
		tq.mu.Lock()
		shouldClose := tq.promoterOn
		done := tq.done
		tq.mu.Unlock()

		if shouldClose {
			tq.quitOnce.Do(func() { close(tq.quit) })
			<-done
		}
	}

	return nil
}

func (a *memoryAdapter) Name() string { return "memory" }

func (a *memoryAdapter) topic(name string) (*topicQueue, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	t, ok := a.topics[name]
	if ok {
		return t, nil
	}

	if a.closed {
		return nil, queue.ErrClosed
	}

	t = newTopicQueue()
	a.topics[name] = t

	return t, nil
}

func (a *memoryAdapter) sweepTopic(name string, t *topicQueue) {
	a.mu.Lock()
	if a.topics[name] == t {
		t.mu.Lock()
		if len(t.ready) == 0 && len(t.delayed) == 0 && len(t.inflight) == 0 && t.penders == 0 {
			if t.promoterOn {
				t.quitOnce.Do(func() { close(t.quit) })
			}

			delete(a.topics, name)
		}
		t.mu.Unlock()
	}
	a.mu.Unlock()
}

func (a *memoryAdapter) waitForSpace(ctx context.Context, t *topicQueue) error {
	return a.waitForCapacity(ctx, t, func() int {
		t.mu.Lock()
		n := len(t.ready)
		t.mu.Unlock()

		return n
	})
}

func (a *memoryAdapter) waitForCapacity(ctx context.Context, t *topicQueue, count func() int) error {
	poll := time.NewTimer(50 * time.Millisecond)
	defer poll.Stop()

	for {
		if count() < a.buffer {
			return nil
		}

		poll.Stop()

		poll.Reset(50 * time.Millisecond)

		select {
		case <-ctx.Done():
			return fmt.Errorf("queue: failed to wait for queue space %w", ctx.Err())
		case <-t.spaceCh:
		case <-poll.C:
		}
	}
}

func (a *memoryAdapter) waitForDelayedSpace(ctx context.Context, t *topicQueue) error {
	return a.waitForCapacity(ctx, t, func() int {
		t.mu.Lock()
		n := len(t.ready) + len(t.delayed)
		t.mu.Unlock()

		return n
	})
}

func (a *memoryAdapter) mountTopicLocked(name string, t *topicQueue) *topicQueue {
	if existing, ok := a.topics[name]; ok {
		if existing == t {
			return t
		}

		return existing
	}

	a.topics[name] = t

	return t
}

func (a *memoryAdapter) getTopic(name string) *topicQueue {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.topics[name]
}

func (a *memoryAdapter) popWithTopic(ctx context.Context, topic string, t *topicQueue) (queue.Message, error) {
	pollTimer := time.NewTimer(a.pollTimeout)

	defer pollTimer.Stop()

	for {
		t.reclaimExpired(a.visibilityTimeout)

		if msg, ok := t.tryPopReady(a.visibilityTimeout); ok {
			t.signalSpace()

			return msg, nil
		}

		t.mu.Lock()
		t.penders++
		t.mu.Unlock()

		select {
		case <-ctx.Done():
			t.mu.Lock()
			t.penders--
			t.mu.Unlock()

			return queue.Message{}, fmt.Errorf("queue: pop cancelled: %w", ctx.Err())

		case <-t.notify:
			t.mu.Lock()
			t.penders--
			t.mu.Unlock()
		case <-pollTimer.C:
			t.mu.Lock()
			t.penders--
			t.mu.Unlock()
			a.sweepTopic(topic, t)

			return queue.Message{}, &queue.EmptyError{Topic: topic}
		}
	}
}
