package memory

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/eventbus"
)

func coverMustBus(t *testing.T, opts eventbus.Options) *bus {
	t.Helper()

	mb, err := newBus(opts)
	if err != nil {
		t.Fatalf("newBus: %v", err)
	}

	return mb
}

func coverWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func TestCoverNewDefaults(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{})
	defer mb.Close()

	if mb.buffer != eventbus.DefaultBufferSize {
		t.Errorf("buffer = %d want %d", mb.buffer, eventbus.DefaultBufferSize)
	}

	if cap(mb.sem) != eventbus.DefaultMaxHandlers {
		t.Errorf("sem cap = %d want %d", cap(mb.sem), eventbus.DefaultMaxHandlers)
	}

	if mb.handlerTimeout != eventbus.DefaultHandlerTimeout {
		t.Errorf("handlerTimeout = %v want %v", mb.handlerTimeout, eventbus.DefaultHandlerTimeout)
	}

	if mb.closeTimeout != eventbus.DefaultCloseTimeout {
		t.Errorf("closeTimeout = %v want %v", mb.closeTimeout, eventbus.DefaultCloseTimeout)
	}

	if mb.topics == nil {
		t.Error("topics is nil")
	}
}

func TestCoverNewExplicit(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     7,
		MaxHandlers:    3,
		HandlerTimeout: time.Second,
		CloseTimeout:   2 * time.Second,
		OnPanic:        func(string, eventbus.Message, any) {},
	})
	defer mb.Close()

	if mb.buffer != 7 {
		t.Errorf("buffer = %d want 7", mb.buffer)
	}

	if cap(mb.sem) != 3 {
		t.Errorf("sem cap = %d want 3", cap(mb.sem))
	}

	if mb.handlerTimeout != time.Second {
		t.Errorf("handlerTimeout = %v want 1s", mb.handlerTimeout)
	}

	if mb.closeTimeout != 2*time.Second {
		t.Errorf("closeTimeout = %v want 2s", mb.closeTimeout)
	}

	if mb.onPanic == nil {
		t.Error("onPanic is nil")
	}
}

func TestCoverSubscribeClosedAfterLock(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     4,
		MaxHandlers:    4,
		HandlerTimeout: time.Second,
		CloseTimeout:   time.Second,
	})
	defer mb.Close()

	mb.mu.Lock()

	subErrCh := make(chan error, 1)

	go func() {
		_, subErr := mb.Subscribe(t.Context(), "t", func(context.Context, eventbus.Message) {})
		subErrCh <- subErr
	}()

	// The test holds b.mu, so the goroutine blocks on it after the
	// pre-lock closed check; closing underneath exercises the post-lock
	// closed check deterministically without any timing wait.
	mb.closed.Store(true)
	mb.mu.Unlock()

	select {
	case gotErr := <-subErrCh:
		if !errors.Is(gotErr, eventbus.ErrClosed) {
			t.Fatalf("Subscribe err = %v want ErrClosed", gotErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Subscribe did not return after close")
	}
}

func TestCoverUnsubDrainBuffered(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     8,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	defer mb.Close()

	ctx := t.Context()
	release := make(chan struct{})

	var entered atomic.Bool

	unsub, subErr := mb.Subscribe(ctx, "drain", func(context.Context, eventbus.Message) {
		entered.Store(true)
		<-release
	})
	if subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "drain", eventbus.NewPayload([]byte("first")), nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	coverWaitFor(t, "handler to start", entered.Load)

	// Forward is stuck in the handler; these stay buffered in sub.ch, so
	// unsub must drain them.
	for range 4 {
		if pubErr := mb.Publish(ctx, "drain", eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
			t.Fatalf("Publish: %v", pubErr)
		}
	}

	unsub()
	close(release)
}

func TestCoverCloseDrainsInflight(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     8,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})

	release := make(chan struct{})
	defer close(release)

	ctx := t.Context()

	var entered atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "inflight", func(context.Context, eventbus.Message) {
		entered.Store(true)
		<-release
	}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "inflight", eventbus.NewPayload([]byte("first")), nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	coverWaitFor(t, "handler to start", entered.Load)

	// Buffered while forward is stuck in the handler. Close does not drain,
	// so the forward drain-defer must drain them on exit.
	for range 5 {
		if pubErr := mb.Publish(ctx, "inflight", eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
			t.Fatalf("Publish: %v", pubErr)
		}
	}

	if closeErr := mb.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
}

func TestCoverSemVsDone(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     16,
		MaxHandlers:    1,
		HandlerTimeout: 5 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	defer mb.Close()

	ctx := t.Context()
	release := make(chan struct{})
	defer close(release)

	var aEntered atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "sem-a", func(context.Context, eventbus.Message) {
		aEntered.Store(true)
		<-release
	}); subErr != nil {
		t.Fatalf("Subscribe sem-a: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "sem-a", eventbus.NewPayload([]byte("a")), nil); pubErr != nil {
		t.Fatalf("Publish sem-a: %v", pubErr)
	}

	// sem-a handler now holds the single sem slot.
	coverWaitFor(t, "sem-a handler to start", aEntered.Load)

	gotB := make(chan eventbus.Message, 4)

	unsubB, subErr := mb.Subscribe(ctx, "sem-b", func(_ context.Context, m eventbus.Message) {
		gotB <- m
	})
	if subErr != nil {
		t.Fatalf("Subscribe sem-b: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "sem-b", eventbus.NewPayload([]byte("b")), nil); pubErr != nil {
		t.Fatalf("Publish sem-b: %v", pubErr)
	}

	// sem-b forward takes the message and blocks acquiring sem; wait until
	// the message leaves the buffer (forward is parked on the sem), then
	// unsub mid-wait so it returns via <-sub.done instead of the sem.
	coverWaitFor(t, "sem-b forward to take message", func() bool {
		mb.mu.RLock()
		defer mb.mu.RUnlock()
		subs := mb.topics["sem-b"]
		if len(subs) != 1 {
			return false
		}
		return len(subs[0].ch) == 0
	})
	unsubB()

	select {
	case m := <-gotB:
		t.Fatalf("sem-b received after unsubscribe: %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCoverHandlerTimeout(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     4,
		MaxHandlers:    4,
		HandlerTimeout: 50 * time.Millisecond,
		CloseTimeout:   5 * time.Second,
	})
	defer mb.Close()

	ctx := t.Context()

	var cancelled atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "slow", func(c context.Context, _ eventbus.Message) {
		select {
		case <-c.Done():
			cancelled.Store(true)
		case <-time.After(300 * time.Millisecond):
		}
	}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "slow", eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	coverWaitFor(t, "handler timeout", cancelled.Load)
}

func TestCoverPanicNilOnPanic(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     8,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	defer mb.Close()

	ctx := t.Context()
	next := make(chan eventbus.Message, 4)

	var once atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "panic", func(_ context.Context, m eventbus.Message) {
		if once.CompareAndSwap(false, true) {
			panic("boom")
		}

		next <- m
	}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "panic", eventbus.NewPayload([]byte("1")), nil); pubErr != nil {
		t.Fatalf("Publish 1: %v", pubErr)
	}

	if pubErr := mb.Publish(ctx, "panic", eventbus.NewPayload([]byte("2")), nil); pubErr != nil {
		t.Fatalf("Publish 2: %v", pubErr)
	}

	select {
	case m := <-next:
		if string(m.Payload) != "2" {
			t.Fatalf("got %q want %q", m.Payload, "2")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message after panic")
	}
}

func TestCoverCloseTimeout(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     4,
		MaxHandlers:    4,
		HandlerTimeout: 5 * time.Second,
		CloseTimeout:   20 * time.Millisecond,
	})

	release := make(chan struct{})
	defer close(release)

	ctx := t.Context()

	var entered atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "stuck", func(context.Context, eventbus.Message) {
		entered.Store(true)
		<-release
	}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "stuck", eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	coverWaitFor(t, "handler to start", entered.Load)

	// Close always closes sub.done, so forwards exit promptly even with a
	// stuck handler; hold the WaitGroup open directly to exercise the
	// close-timeout path deterministically.
	mb.wg.Add(1)
	defer mb.wg.Done()

	start := time.Now()
	closeErr := mb.Close()
	elapsed := time.Since(start)

	if closeErr != nil {
		t.Fatalf("Close err = %v want nil (timeout abandons, like redis)", closeErr)
	}

	if elapsed >= 2*time.Second {
		t.Fatalf("Close took %v want < 2s", elapsed)
	}

	if secondErr := mb.Close(); secondErr != nil {
		t.Fatalf("second Close err = %v want nil", secondErr)
	}
}

func TestCoverDroppedCounter(t *testing.T) {
	mb := coverMustBus(t, eventbus.Options{
		BufferSize:     1,
		MaxHandlers:    4,
		HandlerTimeout: 2 * time.Second,
		CloseTimeout:   5 * time.Second,
	})
	defer mb.Close()

	ctx := t.Context()
	release := make(chan struct{})
	defer close(release)

	var entered atomic.Bool

	if _, subErr := mb.Subscribe(ctx, "drop", func(context.Context, eventbus.Message) {
		entered.Store(true)
		<-release
	}); subErr != nil {
		t.Fatalf("Subscribe: %v", subErr)
	}

	if pubErr := mb.Publish(ctx, "drop", eventbus.NewPayload([]byte("first")), nil); pubErr != nil {
		t.Fatalf("Publish: %v", pubErr)
	}

	coverWaitFor(t, "handler to start", entered.Load)

	// Forward is stuck in the handler; buffer holds 1, the rest drop.
	for range 16 {
		if pubErr := mb.Publish(ctx, "drop", eventbus.NewPayload([]byte("x")), nil); pubErr != nil {
			t.Fatalf("Publish: %v", pubErr)
		}
	}

	mb.mu.RLock()
	subs := mb.topics["drop"]
	mb.mu.RUnlock()

	if len(subs) != 1 {
		t.Fatalf("got %d subs want 1", len(subs))
	}

	if got := subs[0].Dropped(); got == 0 {
		t.Fatal("Dropped() = 0 want > 0")
	}
}
