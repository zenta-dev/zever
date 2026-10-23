package eventbus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPullCoverInnerSubscribeError(t *testing.T) {
	t.Parallel()

	inner := newFakeBus()
	inner.closed = true

	b := Wrap(inner)

	_, err := b.SubscribeChan(t.Context(), "t-inner-err", 1)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed from inner Subscribe", err)
	}
}

// blockingPusher parks inside Subscribe until released, so the test can set
// pullBus.closed in the window between inner-Subscribe success and map insert.
type blockingPusher struct {
	fakeBus
	entered  chan struct{}
	release  chan struct{}
	unsubbed chan struct{}
	once     sync.Once
}

func (b *blockingPusher) Subscribe(ctx context.Context, topic string, handler Handler) (func(), error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release

	unsub, err := b.fakeBus.Subscribe(ctx, topic, handler)
	if err != nil {
		return nil, err
	}

	return func() {
		unsub()
		close(b.unsubbed)
	}, nil
}

func TestPullCoverClosedAfterRegister(t *testing.T) {
	t.Parallel()

	inner := &blockingPusher{
		fakeBus:  fakeBus{subs: make(map[string][]Handler)},
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		unsubbed: make(chan struct{}),
	}
	b := Wrap(inner)
	pb, ok := b.(*pullBus)
	if !ok {
		t.Fatalf("bus = %T, want *pullBus", b)
	}

	type result struct {
		ch  <-chan Message
		err error
	}
	done := make(chan result, 1)
	go func() {
		ch, err := b.SubscribeChan(t.Context(), "t-race", 1)
		done <- result{ch: ch, err: err}
	}()

	select {
	case <-inner.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("inner Subscribe never entered")
	}

	// Close the pull bus while inner Subscribe is parked: on release the
	// map-insert section must take the closed cleanup path.
	pb.closed.Store(true)
	close(inner.release)

	var r result
	select {
	case r = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeChan never returned")
	}

	if !errors.Is(r.err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", r.err)
	}
	if r.ch != nil {
		t.Fatalf("ch = %v, want nil on closed cleanup path", r.ch)
	}

	select {
	case <-inner.unsubbed:
	case <-time.After(5 * time.Second):
		t.Fatal("inner unsubscribe not called on closed cleanup path")
	}

	pb.mu.Lock()
	n := len(pb.subs)
	pb.mu.Unlock()
	if n != 0 {
		t.Fatalf("subs = %d, want 0 after closed cleanup", n)
	}
}
