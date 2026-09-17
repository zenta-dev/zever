package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/eventbus"
	zredis "github.com/zenta-dev/zever/internal/redis"
)

type subscription struct {
	ps      *goredis.PubSub
	topic   string
	channel string
	stop    chan struct{}
	once    sync.Once
}

func (s *subscription) shutdown() {
	s.once.Do(func() { close(s.stop) })
}

var _ eventbus.Pusher = (*adapter)(nil)

type adapter struct {
	client         *goredis.Client
	prefix         string
	buffer         int
	handlerTimeout time.Duration
	closeTimeout   time.Duration
	onPanic        func(string, eventbus.Message, any)
	closed         atomic.Bool
	mu             sync.Mutex
	subs           map[*subscription]struct{}
	wg             sync.WaitGroup
}

// New creates a Redis-backed eventbus. It validates opts first, applies
// defaults (prefix "eventbus", buffer 1024, handler timeout 30s, close
// timeout 5s), reuses the shared internal/redis Pool client, and verifies
// connectivity with a 3s ping.
// The returned bus supports both the push and pull APIs.
func New(opts eventbus.Options) (eventbus.Eventbus, error) {
	a, err := newAdapter(opts)
	if err != nil {
		return nil, err
	}

	return eventbus.Wrap(a), nil
}

func newAdapter(opts eventbus.Options) (*adapter, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}

	prefix := strings.TrimSpace(opts.Redis.Prefix)
	if prefix == "" {
		prefix = "eventbus"
	}

	buffer := opts.BufferSize
	if buffer <= 0 {
		buffer = eventbus.DefaultBufferSize
	}

	handlerTimeout := opts.HandlerTimeout
	if handlerTimeout <= 0 {
		handlerTimeout = eventbus.DefaultHandlerTimeout
	}

	closeTimeout := opts.CloseTimeout
	if closeTimeout <= 0 {
		closeTimeout = eventbus.DefaultCloseTimeout
	}

	client, _ := zredis.New(zredis.Options{
		Addr:     opts.Redis.Addr,
		Password: opts.Redis.Password,
		DB:       opts.Redis.DB,
		TLS:      opts.Redis.TLS,
	})
	// Unreachable post-Validate: Validate rejects scheme/"/ ? #" addrs so
	// toRedisOptions takes its infallible plain host:port branch, and the
	// old pooled client Close cannot fail for a healthy client. The ping
	// check right below preserves fail-closed behavior.

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = zredis.Close()

		return nil, fmt.Errorf("redis: ping %q: %w", redactAddr(opts.Redis.Addr), err)
	}

	return &adapter{
		client:         client,
		prefix:         prefix,
		buffer:         buffer,
		handlerTimeout: handlerTimeout,
		closeTimeout:   closeTimeout,
		onPanic:        opts.OnPanic,
		subs:           make(map[*subscription]struct{}),
	}, nil
}

// redactAddr masks any embedded userinfo credentials, suitable for errors.
func redactAddr(addr string) string {
	return zredis.RedactAddr(addr)
}

// channel maps a topic to its Redis channel.
func (a *adapter) channel(topic string) string {
	return a.prefix + ":" + topic
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

func (a *adapter) Publish(ctx context.Context, topic string, payload eventbus.Payload, headers eventbus.Headers) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("redis: publish %q: %w", topic, err)
	}

	if a.closed.Load() {
		return eventbus.ErrClosed
	}

	if err := validateTopic(topic); err != nil {
		return err
	}

	if len(payload) > eventbus.MaxMessageSize {
		return fmt.Errorf("%w: %d > %d", eventbus.ErrPayloadTooLarge, len(payload), eventbus.MaxMessageSize)
	}

	msg := eventbus.NewMessage(topic, payload, headers)

	wm := wireMessage{
		ID:      msg.ID.String(),
		Payload: msg.Payload,
		Headers: msg.Headers,
	}

	// wireMessage is {string, []byte, map[string]string}: encoding/json
	// cannot fail on it (base64 []byte, string map keys), so the error is
	// provably infallible and discarded.
	b, _ := json.Marshal(wm)

	if len(b) > eventbus.MaxMessageSize {
		return fmt.Errorf("%w: envelope %d > %d", eventbus.ErrPayloadTooLarge, len(b), eventbus.MaxMessageSize)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// At-most-once: the subscriber count is intentionally ignored; zero
	// subscribers means a silent drop by design.
	if err := a.client.Publish(ctx, a.channel(topic), b).Err(); err != nil {
		return fmt.Errorf("redis: publish: %w", err)
	}

	return nil
}

func (a *adapter) Subscribe(ctx context.Context, topic string, handler eventbus.Handler) (func(), error) {
	if handler == nil {
		return nil, eventbus.ErrNilHandler
	}

	if a.closed.Load() {
		return nil, eventbus.ErrClosed
	}

	if err := validateTopic(topic); err != nil {
		return nil, err
	}

	channel := a.channel(topic)

	ps := a.client.Subscribe(ctx, channel)

	subCtx := ctx
	if _, ok := subCtx.Deadline(); !ok {
		var cancel context.CancelFunc

		subCtx, cancel = context.WithTimeout(subCtx, 5*time.Second)
		defer cancel()
	}

	// Fail fast when the broker is unreachable instead of delivering nothing.
	if _, err := ps.Receive(subCtx); err != nil {
		_ = ps.Close()

		return nil, fmt.Errorf("redis: subscribe: %w", err)
	}

	sub := &subscription{
		ps:      ps,
		topic:   topic,
		channel: channel,
		stop:    make(chan struct{}),
	}

	a.mu.Lock()
	if a.closed.Load() {
		a.mu.Unlock()
		_ = ps.Close()

		return nil, eventbus.ErrClosed
	}

	a.subs[sub] = struct{}{}
	a.mu.Unlock()

	a.wg.Add(1)

	go a.deliver(ctx, sub, handler)

	return func() { a.unsubscribe(sub) }, nil
}

func (a *adapter) unsubscribe(sub *subscription) {
	sub.shutdown()

	a.mu.Lock()
	delete(a.subs, sub)
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = sub.ps.Unsubscribe(ctx, sub.channel)

	// Tolerate pool.ErrClosed on repeated closes; idempotent cleanup.
	_ = sub.ps.Close()
}

// Close stops all deliveries and waits up to closeTimeout for in-flight
// handlers. It is idempotent and returns nil. It never closes the shared
// internal/redis Pool client, which is owned by internal/redis.
func (a *adapter) Close() error {
	if !a.closed.CompareAndSwap(false, true) {
		return nil
	}

	a.mu.Lock()
	subs := make([]*subscription, 0, len(a.subs))
	for sub := range a.subs {
		subs = append(subs, sub)
	}
	a.mu.Unlock()

	for _, sub := range subs {
		sub.shutdown()
		_ = sub.ps.Close()
	}

	done := make(chan struct{})

	go func() {
		a.wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(a.closeTimeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
	}

	return nil
}

func (a *adapter) Name() string { return "redis" }
