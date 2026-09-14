package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/eventbus"
)

func testOpts() eventbus.Options {
	return eventbus.Options{
		BufferSize:     16,
		MaxHandlers:    8,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	}
}

func recvTimeout(t *testing.T, ch <-chan eventbus.Message, d time.Duration) eventbus.Message {
	t.Helper()

	select {
	case m := <-ch:
		return m
	case <-time.After(d):
		t.Fatal("timed out waiting for message")
		return eventbus.Message{}
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	if got := b.Name(); got != "memory" {
		t.Fatalf("Name=%q want memory", got)
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(eventbus.Options{BufferSize: -1})
	if err == nil {
		t.Fatal("expected error for negative buffer size")
	}
	if !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
	if !strings.HasPrefix(err.Error(), "memory: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "memory: ")
	}
}

func TestFanout(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()

	const n = 3

	chans := make([]chan eventbus.Message, n)

	for i := range chans {
		chans[i] = make(chan eventbus.Message, 4)

		if _, err := b.Subscribe(ctx, "orders", func(_ context.Context, m eventbus.Message) {
			chans[i] <- m
		}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}

	if err := b.Publish(ctx, "orders", eventbus.NewPayload([]byte("hi")), eventbus.NewHeaders(map[string]string{"k": "v"})); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range chans {
		m := recvTimeout(t, ch, 3*time.Second)
		if m.Topic != "orders" || string(m.Payload) != "hi" || m.Headers["k"] != "v" {
			t.Fatalf("sub %d got %+v", i, m)
		}
	}
}

func TestOrderingPerSub(t *testing.T) {
	t.Parallel()

	b, err := New(eventbus.Options{
		BufferSize:     64,
		MaxHandlers:    8,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	got := make(chan eventbus.Message, 32)

	if _, err := b.Subscribe(ctx, "seq", func(_ context.Context, m eventbus.Message) {
		got <- m
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	const n = 20

	for i := range n {
		if err := b.Publish(ctx, "seq", eventbus.NewPayload([]byte{byte(i)}), nil); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	for i := range n {
		m := recvTimeout(t, got, 3*time.Second)
		if len(m.Payload) != 1 || m.Payload[0] != byte(i) {
			t.Fatalf("position %d got %v", i, m.Payload)
		}
	}
}

func TestDropIsolation(t *testing.T) {
	t.Parallel()

	mb, err := newBus(eventbus.Options{
		BufferSize:     1,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("newBus: %v", err)
	}

	b := mb

	defer b.Close()

	ctx := context.Background()
	release := make(chan struct{})

	// Slow subscriber: blocks until release is closed.
	if _, err := b.Subscribe(ctx, "t", func(_ context.Context, _ eventbus.Message) {
		<-release
	}); err != nil {
		t.Fatalf("Subscribe slow: %v", err)
	}

	fast := make(chan eventbus.Message, 64)

	if _, err := b.Subscribe(ctx, "t", func(_ context.Context, m eventbus.Message) {
		fast <- m
	}); err != nil {
		t.Fatalf("Subscribe fast: %v", err)
	}

	// Fill buffers: first publish occupies slow handler + buffer, rest drop.
	for range 32 {
		_ = b.Publish(ctx, "t", eventbus.NewPayload([]byte("x")), nil)
	}

	// Fast subscriber must still receive despite slow subscriber being full.
	select {
	case <-fast:
	case <-time.After(3 * time.Second):
		t.Fatal("fast subscriber starved by slow subscriber")
	}

	mb.mu.RLock()
	subs := append([]*subscription(nil), mb.topics["t"]...)
	mb.mu.RUnlock()

	if len(subs) != 2 {
		t.Fatalf("got %d subs want 2", len(subs))
	}

	// subs[0] is the slow subscriber registered first.
	deadline := time.Now().Add(3 * time.Second)
	for subs[0].Dropped() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("slow subscriber Dropped()==0, want >0")
		}

		time.Sleep(5 * time.Millisecond)
	}

	close(release)
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	got := make(chan eventbus.Message, 4)

	unsub, err := b.Subscribe(ctx, "u", func(_ context.Context, m eventbus.Message) {
		got <- m
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	unsub()
	unsub() // idempotent

	if err := b.Publish(ctx, "u", eventbus.NewPayload([]byte("x")), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case m := <-got:
		t.Fatalf("received after unsubscribe: %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestPublishAfterClose(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()

	if err := b.Publish(ctx, "t", eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrClosed) {
		t.Fatalf("Publish after close err=%v want ErrClosed", err)
	}

	if _, err := b.Subscribe(ctx, "t", func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrClosed) {
		t.Fatalf("Subscribe after close err=%v want ErrClosed", err)
	}
}

func TestNilHandler(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	if _, err := b.Subscribe(context.Background(), "t", nil); !errors.Is(err, eventbus.ErrNilHandler) {
		t.Fatalf("err=%v want ErrNilHandler", err)
	}
}

func TestOversizePayload(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	big := make(eventbus.Payload, eventbus.MaxMessageSize+1)
	err = b.Publish(context.Background(), "t", big, nil)
	if !errors.Is(err, eventbus.ErrPayloadTooLarge) {
		t.Fatalf("err=%v want ErrPayloadTooLarge", err)
	}
}

func TestInvalidTopic(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()

	if err := b.Publish(ctx, "", eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Fatalf("empty topic Publish err=%v want ErrInvalidOptions", err)
	}

	long := strings.Repeat("a", eventbus.MaxTopicLen+1)
	if err := b.Publish(ctx, long, eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Fatalf("long topic Publish err=%v want ErrInvalidOptions", err)
	}

	if _, err := b.Subscribe(ctx, "", func(context.Context, eventbus.Message) {}); !errors.Is(err, eventbus.ErrInvalidOptions) {
		t.Fatalf("empty topic Subscribe err=%v want ErrInvalidOptions", err)
	}
}

func TestPublishCancelledContext(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := b.Publish(ctx, "t", eventbus.NewPayload([]byte("x")), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}

func TestPanicRecovered(t *testing.T) {
	t.Parallel()

	var panics atomic.Int64

	b, err := New(eventbus.Options{
		BufferSize:     8,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
		OnPanic: func(_ string, _ eventbus.Message, _ any) {
			panics.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	next := make(chan eventbus.Message, 4)
	var once atomic.Bool

	if _, err := b.Subscribe(ctx, "p", func(_ context.Context, m eventbus.Message) {
		if once.CompareAndSwap(false, true) {
			panic("boom")
		}

		next <- m
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := b.Publish(ctx, "p", eventbus.NewPayload([]byte("1")), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if err := b.Publish(ctx, "p", eventbus.NewPayload([]byte("2")), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	m := recvTimeout(t, next, 3*time.Second)
	if string(m.Payload) != "2" {
		t.Fatalf("got %q want %q", m.Payload, "2")
	}

	deadline := time.Now().Add(3 * time.Second)
	for panics.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("OnPanic not called")
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	for range 4 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 25 {
				_ = b.Publish(ctx, "c", eventbus.NewPayload([]byte("x")), nil)
			}
		}()
	}

	for range 4 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			unsub, err := b.Subscribe(ctx, "c", func(context.Context, eventbus.Message) {})
			if err != nil {
				return
			}

			time.Sleep(time.Millisecond)
			unsub()
		}()
	}

	wg.Wait()
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	b, err := New(testOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
