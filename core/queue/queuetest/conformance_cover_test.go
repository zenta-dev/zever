package queuetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

// stubQueue is a scriptable in-memory queue.Queue double with ready
// lists, inflight tracking, delayed delivery, and per-method error
// fields driving each conformance failure branch.
type stubQueue struct {
	mu sync.Mutex

	ready       map[string][]queue.Message
	inflight    map[queue.MessageID]queue.Message
	delayed     []delayedMsg
	pushErr     error
	pushCalls   int
	pushFailOn  int
	pushFailErr error
	popErr      error
	popErrTopic map[string]error
	ackErr      error
	ackCalls    int
	ackFailOn   int
	ackFailErr  error
	nackErr     error
	lenErr      error
	lenCalls    int
	lenFailOn   int
	lenFailVal  int64
	emptyErr    error
	emptyCalls  int
	emptyFailOn int
	isEmptyLie  map[string]bool
	closeErr    error
	closeAg     error
	closes      int
	closed      bool
	name        string
	// noDelay treats delayed pushes as never deliverable.
	noDelay bool
	// dropNack drops the message even when requeue is true.
	dropNack bool
}

type delayedMsg struct {
	topic string
	msg   queue.Message
	due   time.Time
}

func healthyStubQueue() *stubQueue {
	return &stubQueue{
		ready:    map[string][]queue.Message{},
		inflight: map[queue.MessageID]queue.Message{},
		name:     "stub",
	}
}

func newMsg(topic string, payload string) queue.Message {
	return queue.NewMessage(topic, queue.Payload(payload), nil)
}

func (s *stubQueue) Push(_ context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return queue.ErrClosed
	}
	if s.pushErr != nil {
		return s.pushErr
	}
	s.pushCalls++
	if s.pushFailOn > 0 && s.pushCalls == s.pushFailOn {
		return s.pushFailErr
	}
	m := queue.NewMessage(topic, append(queue.Payload(nil), payload...), headers.Clone())
	s.ready[topic] = append(s.ready[topic], m)
	return nil
}

func (s *stubQueue) PushDelayed(_ context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return queue.ErrClosed
	}
	if s.pushErr != nil {
		return s.pushErr
	}
	m := queue.NewMessage(topic, append(queue.Payload(nil), payload...), headers.Clone())
	if delay <= 0 {
		s.ready[topic] = append(s.ready[topic], m)
		return nil
	}
	if s.noDelay {
		return nil
	}
	s.delayed = append(s.delayed, delayedMsg{topic: topic, msg: m, due: time.Now().Add(delay)})
	return nil
}

func (s *stubQueue) promoteLocked(topic string) {
	now := time.Now()
	kept := s.delayed[:0]
	for _, d := range s.delayed {
		if d.topic == topic && !now.Before(d.due) {
			s.ready[topic] = append(s.ready[topic], d.msg)
		} else {
			kept = append(kept, d)
		}
	}
	s.delayed = kept
}

func (s *stubQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return queue.Message{}, queue.ErrClosed
	}
	select {
	case <-ctx.Done():
		return queue.Message{}, ctx.Err()
	default:
	}
	if s.popErr != nil {
		return queue.Message{}, s.popErr
	}
	if err, ok := s.popErrTopic[topic]; ok && err != nil {
		return queue.Message{}, err
	}
	s.promoteLocked(topic)
	l := s.ready[topic]
	if len(l) == 0 {
		return queue.Message{}, queue.EmptyError{Topic: topic}
	}
	m := l[0].Clone()
	m.Topic = topic
	s.ready[topic] = l[1:]
	s.inflight[m.ID] = m
	out := m.Clone()
	return out, nil
}

func (s *stubQueue) Ack(_ context.Context, msg queue.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return queue.ErrClosed
	}
	if s.ackErr != nil {
		return s.ackErr
	}
	s.ackCalls++
	if s.ackFailOn > 0 && s.ackCalls == s.ackFailOn {
		return s.ackFailErr
	}
	delete(s.inflight, msg.ID)
	return nil
}

func (s *stubQueue) Nack(_ context.Context, msg queue.Message, requeue bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return queue.ErrClosed
	}
	if s.nackErr != nil {
		return s.nackErr
	}
	stored, ok := s.inflight[msg.ID]
	if !ok {
		return nil
	}
	delete(s.inflight, msg.ID)
	if requeue && !s.dropNack {
		stored.Attempt++
		s.ready[stored.Topic] = append(s.ready[stored.Topic], stored)
	}
	return nil
}

func (s *stubQueue) Length(_ context.Context, topic string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, queue.ErrClosed
	}
	if s.lenErr != nil {
		return 0, s.lenErr
	}
	s.lenCalls++
	if s.lenFailOn > 0 && s.lenCalls == s.lenFailOn {
		return s.lenFailVal, nil
	}
	return int64(len(s.ready[topic])), nil
}

func (s *stubQueue) IsEmpty(_ context.Context, topic string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, queue.ErrClosed
	}
	if s.emptyErr != nil {
		return false, s.emptyErr
	}
	s.emptyCalls++
	if s.emptyFailOn > 0 && s.emptyCalls == s.emptyFailOn {
		return true, nil
	}
	if lie, ok := s.isEmptyLie[topic]; ok && lie {
		return false, nil
	}
	return len(s.ready[topic]) == 0, nil
}

func (s *stubQueue) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAg
	}
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = true
	return nil
}

func (s *stubQueue) Name() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.name
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckFIFOSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkFIFO(t.Context(), healthyStubQueue()))
}

func TestCheckFIFOFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkFIFO(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkFIFO() = %v, want wrap of boom", err)
		}
	})

	t.Run("pop error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.popErr = boom

		err := checkFIFO(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Pop(0)") {
			t.Fatalf("checkFIFO() = %v, want Pop error", err)
		}
	})

	t.Run("shape drift", func(t *testing.T) {
		t.Parallel()

		stub := &driftQueue{stub: healthyStubQueue()}

		err := checkFIFO(t.Context(), stub)
		if err == nil {
			t.Fatal("checkFIFO(drift) = nil, want shape errors")
		}
		for _, want := range []string{"Topic", "Payload", "Attempt", "ID is zero"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkFIFO() = %v, want containing %q", err, want)
			}
		}
	})

	t.Run("duplicated id", func(t *testing.T) {
		t.Parallel()

		stub := &dupIDQueue{stub: healthyStubQueue()}

		err := checkFIFO(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "duplicated") {
			t.Fatalf("checkFIFO() = %v, want duplicated error", err)
		}
	})

	t.Run("headers drift", func(t *testing.T) {
		t.Parallel()

		stub := &headersDriftQueue{stub: healthyStubQueue()}

		err := checkFIFO(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Headers[k]") {
			t.Fatalf("checkFIFO() = %v, want Headers error", err)
		}
	})

	t.Run("ack error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.ackErr = boom

		err := checkFIFO(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Ack(0)") {
			t.Fatalf("checkFIFO() = %v, want Ack error", err)
		}
	})

	t.Run("length and drain", func(t *testing.T) {
		t.Parallel()

		stub := &stickyLengthQueue{stub: healthyStubQueue()}

		err := checkFIFO(t.Context(), stub)
		if err == nil {
			t.Fatal("checkFIFO(sticky) = nil, want drain errors")
		}
		for _, want := range []string{"Length()", "drained"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkFIFO() = %v, want containing %q", err, want)
			}
		}
	})
}

// driftQueue corrupts every popped message shape.
type driftQueue struct {
	stub *stubQueue
}

func (d *driftQueue) Push(ctx context.Context, topic string, p queue.Payload, h queue.Headers) error {
	return d.stub.Push(ctx, topic, p, h)
}

func (d *driftQueue) PushDelayed(ctx context.Context, topic string, p queue.Payload, h queue.Headers, delay time.Duration) error {
	return d.stub.PushDelayed(ctx, topic, p, h, delay)
}

func (d *driftQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := d.stub.Pop(ctx, topic)
	if err != nil {
		return m, err
	}
	m.Topic = "wrong"
	m.Payload = queue.Payload("wrong")
	m.Attempt = 9
	m.ID = queue.MessageID{}
	m.Headers = queue.Headers{}
	return m, nil
}

func (d *driftQueue) Ack(_ context.Context, _ queue.Message) error {
	// Ack by draining the real inflight instead: pop already consumed
	// the stub message, so ack the stub's view directly.
	d.stub.mu.Lock()
	for id := range d.stub.inflight {
		delete(d.stub.inflight, id)
		break
	}
	d.stub.mu.Unlock()
	return nil
}

func (d *driftQueue) Nack(ctx context.Context, m queue.Message, requeue bool) error {
	return d.stub.Nack(ctx, m, requeue)
}

func (d *driftQueue) Length(ctx context.Context, topic string) (int64, error) {
	return d.stub.Length(ctx, topic)
}

func (d *driftQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return d.stub.IsEmpty(ctx, topic)
}

func (d *driftQueue) Close() error { return d.stub.Close() }

func (d *driftQueue) Name() string { return d.stub.Name() }

// dupIDQueue returns valid shapes with one fixed ID, so the second Pop
// trips the duplicate branch.
type dupIDQueue struct {
	stub *stubQueue
	id   queue.MessageID
}

func (d *dupIDQueue) Push(ctx context.Context, topic string, p queue.Payload, h queue.Headers) error {
	return d.stub.Push(ctx, topic, p, h)
}

func (d *dupIDQueue) PushDelayed(ctx context.Context, topic string, p queue.Payload, h queue.Headers, delay time.Duration) error {
	return d.stub.PushDelayed(ctx, topic, p, h, delay)
}

func (d *dupIDQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := d.stub.Pop(ctx, topic)
	if err != nil {
		return m, err
	}
	if d.id == (queue.MessageID{}) {
		d.id = m.ID
	}
	m.ID = d.id
	return m, nil
}

func (d *dupIDQueue) Ack(ctx context.Context, m queue.Message) error {
	return d.stub.Ack(ctx, m)
}

func (d *dupIDQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return d.stub.Nack(ctx, m, r)
}

func (d *dupIDQueue) Length(ctx context.Context, topic string) (int64, error) {
	return d.stub.Length(ctx, topic)
}

func (d *dupIDQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return d.stub.IsEmpty(ctx, topic)
}

func (d *dupIDQueue) Close() error { return d.stub.Close() }

func (d *dupIDQueue) Name() string { return d.stub.Name() }

// headersDriftQueue corrupts only the i==1 headers.
type headersDriftQueue struct {
	stub *stubQueue
	n    int
}

func (h *headersDriftQueue) Push(ctx context.Context, topic string, p queue.Payload, hd queue.Headers) error {
	return h.stub.Push(ctx, topic, p, hd)
}

func (h *headersDriftQueue) PushDelayed(ctx context.Context, topic string, p queue.Payload, hd queue.Headers, d time.Duration) error {
	return h.stub.PushDelayed(ctx, topic, p, hd, d)
}

func (h *headersDriftQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := h.stub.Pop(ctx, topic)
	if err != nil {
		return m, err
	}
	h.n++
	if h.n == 2 {
		m.Headers = queue.Headers{"k": "wrong"}
	}
	return m, nil
}

func (h *headersDriftQueue) Ack(ctx context.Context, m queue.Message) error {
	return h.stub.Ack(ctx, m)
}

func (h *headersDriftQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return h.stub.Nack(ctx, m, r)
}

func (h *headersDriftQueue) Length(ctx context.Context, topic string) (int64, error) {
	return h.stub.Length(ctx, topic)
}

func (h *headersDriftQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return h.stub.IsEmpty(ctx, topic)
}

func (h *headersDriftQueue) Close() error { return h.stub.Close() }

func (h *headersDriftQueue) Name() string { return h.stub.Name() }

// stickyLengthQueue reports Length 5 and serves a phantom message on
// drain Pop, tripping both tail branches.
type stickyLengthQueue struct {
	stub *stubQueue
}

func (s *stickyLengthQueue) Push(ctx context.Context, topic string, p queue.Payload, h queue.Headers) error {
	return s.stub.Push(ctx, topic, p, h)
}

func (s *stickyLengthQueue) PushDelayed(ctx context.Context, topic string, p queue.Payload, h queue.Headers, d time.Duration) error {
	return s.stub.PushDelayed(ctx, topic, p, h, d)
}

func (s *stickyLengthQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := s.stub.Pop(ctx, topic)
	if err == nil {
		return m, nil
	}
	return newMsg(topic, "phantom"), nil
}

func (s *stickyLengthQueue) Ack(ctx context.Context, m queue.Message) error {
	return s.stub.Ack(ctx, m)
}

func (s *stickyLengthQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return s.stub.Nack(ctx, m, r)
}

func (s *stickyLengthQueue) Length(context.Context, string) (int64, error) {
	return 5, nil
}

func (s *stickyLengthQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return s.stub.IsEmpty(ctx, topic)
}

func (s *stickyLengthQueue) Close() error { return s.stub.Close() }

func (s *stickyLengthQueue) Name() string { return s.stub.Name() }

func TestCheckEmptySuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkEmpty(t.Context(), healthyStubQueue()))
}

func TestCheckEmptyFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("missing not empty", func(t *testing.T) {
		t.Parallel()

		stub := &phantomQueue{stub: healthyStubQueue()}

		err := checkEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "missing topic") {
			t.Fatalf("checkEmpty() = %v, want missing-topic error", err)
		}
	})

	t.Run("push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkEmpty(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkEmpty() = %v, want wrap of boom", err)
		}
	})

	t.Run("inflight visible", func(t *testing.T) {
		t.Parallel()

		stub := &visibleInflightQueue{stub: healthyStubQueue()}

		err := checkEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "inflight only") {
			t.Fatalf("checkEmpty() = %v, want inflight error", err)
		}
	})

	t.Run("bare empty error", func(t *testing.T) {
		t.Parallel()

		stub := &bareEmptyQueue{stub: healthyStubQueue()}

		err := checkEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "EmptyError") {
			t.Fatalf("checkEmpty() = %v, want EmptyError error", err)
		}
	})

	t.Run("wrong topic", func(t *testing.T) {
		t.Parallel()

		stub := &wrongTopicEmptyQueue{stub: healthyStubQueue()}

		err := checkEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "EmptyError.Topic") {
			t.Fatalf("checkEmpty() = %v, want topic error", err)
		}
	})

	t.Run("cancelled ctx delivers", func(t *testing.T) {
		t.Parallel()

		stub := &ignoreCtxQueue{stub: healthyStubQueue()}

		err := checkEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "cancelled ctx") {
			t.Fatalf("checkEmpty() = %v, want cancelled-ctx error", err)
		}
	})
}

// phantomQueue serves a message for every topic.
type phantomQueue struct {
	stub *stubQueue
}

func (p *phantomQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return p.stub.Push(ctx, topic, pl, h)
}

func (p *phantomQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return p.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (p *phantomQueue) Pop(_ context.Context, topic string) (queue.Message, error) {
	return newMsg(topic, "phantom"), nil
}

func (p *phantomQueue) Ack(ctx context.Context, m queue.Message) error {
	return p.stub.Ack(ctx, m)
}

func (p *phantomQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return p.stub.Nack(ctx, m, r)
}

func (p *phantomQueue) Length(ctx context.Context, topic string) (int64, error) {
	return p.stub.Length(ctx, topic)
}

func (p *phantomQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return p.stub.IsEmpty(ctx, topic)
}

func (p *phantomQueue) Close() error { return p.stub.Close() }

func (p *phantomQueue) Name() string { return p.stub.Name() }

// visibleInflightQueue keeps serving the message after Pop.
type visibleInflightQueue struct {
	stub *stubQueue
	pops int
}

func (v *visibleInflightQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return v.stub.Push(ctx, topic, pl, h)
}

func (v *visibleInflightQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return v.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (v *visibleInflightQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	v.pops++
	if v.pops > 2 {
		// Third pop (inflight-only): replay the message instead of
		// reporting empty.
		return newMsg(topic, "only"), nil
	}
	return v.stub.Pop(ctx, topic)
}

func (v *visibleInflightQueue) Ack(ctx context.Context, m queue.Message) error {
	return v.stub.Ack(ctx, m)
}

func (v *visibleInflightQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return v.stub.Nack(ctx, m, r)
}

func (v *visibleInflightQueue) Length(ctx context.Context, topic string) (int64, error) {
	return v.stub.Length(ctx, topic)
}

func (v *visibleInflightQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return v.stub.IsEmpty(ctx, topic)
}

func (v *visibleInflightQueue) Close() error { return v.stub.Close() }

func (v *visibleInflightQueue) Name() string { return v.stub.Name() }

// bareEmptyQueue reports ErrEmpty without the topic attached.
type bareEmptyQueue struct {
	stub *stubQueue
	pops int
}

func (b *bareEmptyQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return b.stub.Push(ctx, topic, pl, h)
}

func (b *bareEmptyQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return b.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (b *bareEmptyQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	b.pops++
	if b.pops == 1 {
		return queue.Message{}, queue.ErrEmpty
	}
	if b.pops == 2 {
		m, err := b.stub.Pop(ctx, topic)
		// First real pop must succeed; fail loudly otherwise.
		if err != nil {
			return m, err
		}
		return m, nil
	}
	return queue.Message{}, queue.ErrEmpty
}

func (b *bareEmptyQueue) Ack(ctx context.Context, m queue.Message) error {
	return b.stub.Ack(ctx, m)
}

func (b *bareEmptyQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return b.stub.Nack(ctx, m, r)
}

func (b *bareEmptyQueue) Length(ctx context.Context, topic string) (int64, error) {
	return b.stub.Length(ctx, topic)
}

func (b *bareEmptyQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return b.stub.IsEmpty(ctx, topic)
}

func (b *bareEmptyQueue) Close() error { return b.stub.Close() }

func (b *bareEmptyQueue) Name() string { return b.stub.Name() }

// wrongTopicEmptyQueue attaches the wrong topic to EmptyError.
type wrongTopicEmptyQueue struct {
	stub *stubQueue
	pops int
}

func (w *wrongTopicEmptyQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return w.stub.Push(ctx, topic, pl, h)
}

func (w *wrongTopicEmptyQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return w.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (w *wrongTopicEmptyQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	w.pops++
	if w.pops == 1 {
		return queue.Message{}, queue.EmptyError{Topic: "never-used-topic"}
	}
	if w.pops == 2 {
		return w.stub.Pop(ctx, topic)
	}
	return queue.Message{}, queue.EmptyError{Topic: "wrong"}
}

func (w *wrongTopicEmptyQueue) Ack(ctx context.Context, m queue.Message) error {
	return w.stub.Ack(ctx, m)
}

func (w *wrongTopicEmptyQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return w.stub.Nack(ctx, m, r)
}

func (w *wrongTopicEmptyQueue) Length(ctx context.Context, topic string) (int64, error) {
	return w.stub.Length(ctx, topic)
}

func (w *wrongTopicEmptyQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return w.stub.IsEmpty(ctx, topic)
}

func (w *wrongTopicEmptyQueue) Close() error { return w.stub.Close() }

func (w *wrongTopicEmptyQueue) Name() string { return w.stub.Name() }

// ignoreCtxQueue serves from ready even on a cancelled context.
type ignoreCtxQueue struct {
	stub *stubQueue
	pops int
}

func (ig *ignoreCtxQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return ig.stub.Push(ctx, topic, pl, h)
}

func (ig *ignoreCtxQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return ig.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (ig *ignoreCtxQueue) Pop(_ context.Context, topic string) (queue.Message, error) {
	ig.pops++
	if ig.pops < 4 {
		return ig.stub.Pop(context.Background(), topic)
	}
	ig.stub.mu.Lock()
	defer ig.stub.mu.Unlock()

	l := ig.stub.ready[topic]
	if len(l) == 0 {
		// After ack the topic is drained; serve phantom so the
		// cancelled-ctx Pop returns nil error.
		return newMsg(topic, "phantom"), nil
	}
	m := l[0].Clone()
	ig.stub.ready[topic] = l[1:]
	ig.stub.inflight[m.ID] = m
	return m.Clone(), nil
}

func (ig *ignoreCtxQueue) Ack(ctx context.Context, m queue.Message) error {
	return ig.stub.Ack(ctx, m)
}

func (ig *ignoreCtxQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return ig.stub.Nack(ctx, m, r)
}

func (ig *ignoreCtxQueue) Length(ctx context.Context, topic string) (int64, error) {
	return ig.stub.Length(ctx, topic)
}

func (ig *ignoreCtxQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return ig.stub.IsEmpty(ctx, topic)
}

func (ig *ignoreCtxQueue) Close() error { return ig.stub.Close() }

func (ig *ignoreCtxQueue) Name() string { return ig.stub.Name() }

func TestCheckLengthIsEmptySuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkLengthIsEmpty(t.Context(), healthyStubQueue()))
}

func TestCheckLengthIsEmptyFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("first push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkLengthIsEmpty(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkLengthIsEmpty() = %v, want wrap of boom", err)
		}
	})

	t.Run("second push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushFailOn = 2
		stub.pushFailErr = boom

		if err := checkLengthIsEmpty(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkLengthIsEmpty() = %v, want wrap of boom", err)
		}
	})

	t.Run("length want 2", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.lenFailOn = 2
		stub.lenFailVal = 9

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Length() =") {
			t.Fatalf("checkLengthIsEmpty() = %v, want Length error", err)
		}
	})

	t.Run("not empty after push", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.emptyFailOn = 2

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "IsEmpty() =") {
			t.Fatalf("checkLengthIsEmpty() = %v, want IsEmpty error", err)
		}
	})

	t.Run("pop error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.popErr = boom

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Pop() error") {
			t.Fatalf("checkLengthIsEmpty() = %v, want Pop error", err)
		}
	})

	t.Run("ack error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.ackFailOn = 1
		stub.ackFailErr = boom

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Ack() error") {
			t.Fatalf("checkLengthIsEmpty() = %v, want Ack error", err)
		}
	})

	t.Run("length drift", func(t *testing.T) {
		t.Parallel()

		stub := &driftLengthQueue{stub: healthyStubQueue()}

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Length(missing)") {
			t.Fatalf("checkLengthIsEmpty() = %v, want Length error", err)
		}
	})

	t.Run("empty drift", func(t *testing.T) {
		t.Parallel()

		stub := &driftEmptyQueue{stub: healthyStubQueue()}

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "IsEmpty(missing)") {
			t.Fatalf("checkLengthIsEmpty() = %v, want IsEmpty error", err)
		}
	})

	t.Run("inflight counted", func(t *testing.T) {
		t.Parallel()

		stub := &countInflightQueue{stub: healthyStubQueue()}

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Length(inflight)") {
			t.Fatalf("checkLengthIsEmpty() = %v, want inflight error", err)
		}
	})

	t.Run("after ack wrong", func(t *testing.T) {
		t.Parallel()

		stub := &stickyAfterAckQueue{stub: healthyStubQueue()}

		err := checkLengthIsEmpty(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "after ack") {
			t.Fatalf("checkLengthIsEmpty() = %v, want after-ack error", err)
		}
	})
}

// driftLengthQueue misreports the empty topic.
type driftLengthQueue struct {
	stub *stubQueue
}

func (d *driftLengthQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return d.stub.Push(ctx, topic, pl, h)
}

func (d *driftLengthQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, delay time.Duration) error {
	return d.stub.PushDelayed(ctx, topic, pl, h, delay)
}

func (d *driftLengthQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return d.stub.Pop(ctx, topic)
}

func (d *driftLengthQueue) Ack(ctx context.Context, m queue.Message) error {
	return d.stub.Ack(ctx, m)
}

func (d *driftLengthQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return d.stub.Nack(ctx, m, r)
}

func (d *driftLengthQueue) Length(context.Context, string) (int64, error) { return 3, nil }

func (d *driftLengthQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return d.stub.IsEmpty(ctx, topic)
}

func (d *driftLengthQueue) Close() error { return d.stub.Close() }

func (d *driftLengthQueue) Name() string { return d.stub.Name() }

// driftEmptyQueue misreports the empty topic as non-empty.
type driftEmptyQueue struct {
	stub *stubQueue
}

func (d *driftEmptyQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return d.stub.Push(ctx, topic, pl, h)
}

func (d *driftEmptyQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, delay time.Duration) error {
	return d.stub.PushDelayed(ctx, topic, pl, h, delay)
}

func (d *driftEmptyQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return d.stub.Pop(ctx, topic)
}

func (d *driftEmptyQueue) Ack(ctx context.Context, m queue.Message) error {
	return d.stub.Ack(ctx, m)
}

func (d *driftEmptyQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return d.stub.Nack(ctx, m, r)
}

func (d *driftEmptyQueue) Length(ctx context.Context, topic string) (int64, error) {
	return d.stub.Length(ctx, topic)
}

func (d *driftEmptyQueue) IsEmpty(context.Context, string) (bool, error) { return false, nil }

func (d *driftEmptyQueue) Close() error { return d.stub.Close() }

func (d *driftEmptyQueue) Name() string { return d.stub.Name() }

// countInflightQueue includes inflight messages in Length.
type countInflightQueue struct {
	stub *stubQueue
}

func (c *countInflightQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return c.stub.Push(ctx, topic, pl, h)
}

func (c *countInflightQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return c.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (c *countInflightQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return c.stub.Pop(ctx, topic)
}

func (c *countInflightQueue) Ack(ctx context.Context, m queue.Message) error {
	return c.stub.Ack(ctx, m)
}

func (c *countInflightQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return c.stub.Nack(ctx, m, r)
}

func (c *countInflightQueue) Length(ctx context.Context, topic string) (int64, error) {
	n, err := c.stub.Length(ctx, topic)
	if err != nil {
		return n, err
	}
	c.stub.mu.Lock()
	defer c.stub.mu.Unlock()

	return n + int64(len(c.stub.inflight)), nil
}

func (c *countInflightQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return c.stub.IsEmpty(ctx, topic)
}

func (c *countInflightQueue) Close() error { return c.stub.Close() }

func (c *countInflightQueue) Name() string { return c.stub.Name() }

// stickyAfterAckQueue keeps Length at 2 after ack.
type stickyAfterAckQueue struct {
	stub *stubQueue
	acks int
}

func (s *stickyAfterAckQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return s.stub.Push(ctx, topic, pl, h)
}

func (s *stickyAfterAckQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return s.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (s *stickyAfterAckQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return s.stub.Pop(ctx, topic)
}

func (s *stickyAfterAckQueue) Ack(ctx context.Context, m queue.Message) error {
	s.acks++
	return s.stub.Ack(ctx, m)
}

func (s *stickyAfterAckQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return s.stub.Nack(ctx, m, r)
}

func (s *stickyAfterAckQueue) Length(ctx context.Context, topic string) (int64, error) {
	if s.acks > 0 {
		return 2, nil
	}
	return s.stub.Length(ctx, topic)
}

func (s *stickyAfterAckQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return s.stub.IsEmpty(ctx, topic)
}

func (s *stickyAfterAckQueue) Close() error { return s.stub.Close() }

func (s *stickyAfterAckQueue) Name() string { return s.stub.Name() }

func TestCheckAckSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkAck(t.Context(), healthyStubQueue()))
}

func TestCheckAckFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkAck(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkAck() = %v, want wrap of boom", err)
		}
	})

	t.Run("ack not idempotent", func(t *testing.T) {
		t.Parallel()

		stub := &failSecondAckQueue{stub: healthyStubQueue()}

		err := checkAck(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Ack(again)") {
			t.Fatalf("checkAck() = %v, want Ack(again) error", err)
		}
	})

	t.Run("message returns", func(t *testing.T) {
		t.Parallel()

		stub := &resurrectQueue{stub: healthyStubQueue()}

		err := checkAck(t.Context(), stub)
		if err == nil {
			t.Fatal("checkAck(resurrect) = nil, want errors")
		}
		for _, want := range []string{"after ack", "Length(after ack)", "IsEmpty(after ack)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkAck() = %v, want containing %q", err, want)
			}
		}
	})
}

// failSecondAckQueue fails the second Ack.
type failSecondAckQueue struct {
	stub  *stubQueue
	calls int
}

func (f *failSecondAckQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return f.stub.Push(ctx, topic, pl, h)
}

func (f *failSecondAckQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return f.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (f *failSecondAckQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return f.stub.Pop(ctx, topic)
}

func (f *failSecondAckQueue) Ack(ctx context.Context, m queue.Message) error {
	f.calls++
	if f.calls == 2 {
		return errors.New("queuetest: ack-again boom")
	}
	return f.stub.Ack(ctx, m)
}

func (f *failSecondAckQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return f.stub.Nack(ctx, m, r)
}

func (f *failSecondAckQueue) Length(ctx context.Context, topic string) (int64, error) {
	return f.stub.Length(ctx, topic)
}

func (f *failSecondAckQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return f.stub.IsEmpty(ctx, topic)
}

func (f *failSecondAckQueue) Close() error { return f.stub.Close() }

func (f *failSecondAckQueue) Name() string { return f.stub.Name() }

// resurrectQueue requeues on Ack, so the message returns.
type resurrectQueue struct {
	stub *stubQueue
}

func (r *resurrectQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return r.stub.Push(ctx, topic, pl, h)
}

func (r *resurrectQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return r.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (r *resurrectQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return r.stub.Pop(ctx, topic)
}

func (r *resurrectQueue) Ack(_ context.Context, m queue.Message) error {
	r.stub.mu.Lock()
	defer r.stub.mu.Unlock()

	m.Attempt++
	r.stub.ready[m.Topic] = append(r.stub.ready[m.Topic], m)
	return nil
}

func (r *resurrectQueue) Nack(ctx context.Context, m queue.Message, r2 bool) error {
	return r.stub.Nack(ctx, m, r2)
}

func (r *resurrectQueue) Length(ctx context.Context, topic string) (int64, error) {
	return r.stub.Length(ctx, topic)
}

func (r *resurrectQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return r.stub.IsEmpty(ctx, topic)
}

func (r *resurrectQueue) Close() error { return r.stub.Close() }

func (r *resurrectQueue) Name() string { return r.stub.Name() }

func TestCheckNackRequeueSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkNackRequeue(t.Context(), healthyStubQueue()))
}

func TestCheckNackRequeueFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("push error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkNackRequeue(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkNackRequeue() = %v, want wrap of boom", err)
		}
	})

	t.Run("nack error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.nackErr = boom

		err := checkNackRequeue(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Nack(requeue)") {
			t.Fatalf("checkNackRequeue() = %v, want Nack error", err)
		}
	})

	t.Run("drop on requeue", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.dropNack = true

		err := checkNackRequeue(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "redelivered") {
			t.Fatalf("checkNackRequeue() = %v, want redelivery error", err)
		}
	})

	t.Run("aliased redelivery", func(t *testing.T) {
		t.Parallel()

		stub := &aliasNackQueue{stub: healthyStubQueue()}

		err := checkNackRequeue(t.Context(), stub)
		if err == nil {
			t.Fatal("checkNackRequeue(alias) = nil, want stored-copy errors")
		}
		for _, want := range []string{"stored copy", "Attempt"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkNackRequeue() = %v, want containing %q", err, want)
			}
		}
	})
}

// aliasNackQueue requeues the caller-mutated copy without bumping attempt.
type aliasNackQueue struct {
	stub *stubQueue
	orig queue.Message
}

func (a *aliasNackQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return a.stub.Push(ctx, topic, pl, h)
}

func (a *aliasNackQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return a.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (a *aliasNackQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := a.stub.Pop(ctx, topic)
	if err != nil {
		return m, err
	}
	a.orig = m
	return m, nil
}

func (a *aliasNackQueue) Ack(ctx context.Context, m queue.Message) error {
	return a.stub.Ack(ctx, m)
}

func (a *aliasNackQueue) Nack(_ context.Context, m queue.Message, _ bool) error {
	a.stub.mu.Lock()
	defer a.stub.mu.Unlock()

	delete(a.stub.inflight, m.ID)
	a.stub.ready[m.Topic] = append(a.stub.ready[m.Topic], m)
	return nil
}

func (a *aliasNackQueue) Length(ctx context.Context, topic string) (int64, error) {
	return a.stub.Length(ctx, topic)
}

func (a *aliasNackQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return a.stub.IsEmpty(ctx, topic)
}

func (a *aliasNackQueue) Close() error { return a.stub.Close() }

func (a *aliasNackQueue) Name() string { return a.stub.Name() }

func TestCheckNackDropSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkNackDrop(t.Context(), healthyStubQueue()))
}

func TestCheckNackDropFailures(t *testing.T) {
	t.Parallel()

	t.Run("drop keeps", func(t *testing.T) {
		t.Parallel()

		stub := &keepOnDropQueue{stub: healthyStubQueue()}

		err := checkNackDrop(t.Context(), stub)
		if err == nil {
			t.Fatal("checkNackDrop(keep) = nil, want errors")
		}
		for _, want := range []string{"after drop", "Length(after drop)", "IsEmpty(after drop)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkNackDrop() = %v, want containing %q", err, want)
			}
		}
	})
}

// keepOnDropQueue requeues even on drop.
type keepOnDropQueue struct {
	stub *stubQueue
}

func (k *keepOnDropQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return k.stub.Push(ctx, topic, pl, h)
}

func (k *keepOnDropQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return k.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (k *keepOnDropQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return k.stub.Pop(ctx, topic)
}

func (k *keepOnDropQueue) Ack(ctx context.Context, m queue.Message) error {
	return k.stub.Ack(ctx, m)
}

func (k *keepOnDropQueue) Nack(_ context.Context, m queue.Message, _ bool) error {
	k.stub.mu.Lock()
	defer k.stub.mu.Unlock()

	delete(k.stub.inflight, m.ID)
	m.Attempt++
	k.stub.ready[m.Topic] = append(k.stub.ready[m.Topic], m)
	return nil
}

func (k *keepOnDropQueue) Length(context.Context, string) (int64, error) {
	return 9, nil
}

func (k *keepOnDropQueue) IsEmpty(context.Context, string) (bool, error) {
	return false, nil
}

func (k *keepOnDropQueue) Close() error { return k.stub.Close() }

func (k *keepOnDropQueue) Name() string { return k.stub.Name() }

func TestCheckDelayedSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkDelayed(t.Context(), healthyStubQueue(), 60*time.Millisecond, 2*time.Second))
}

func TestCheckDelayedFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("push delayed error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkDelayed(t.Context(), stub, time.Millisecond, time.Second); !errors.Is(err, boom) {
			t.Fatalf("checkDelayed() = %v, want wrap of boom", err)
		}
	})

	t.Run("never delivers", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.noDelay = true

		err := checkDelayed(t.Context(), stub, time.Hour, 80*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "condition not met") {
			t.Fatalf("checkDelayed() = %v, want poll-timeout error", err)
		}
	})

	t.Run("wrong payload", func(t *testing.T) {
		t.Parallel()

		stub := &wrongDelayedQueue{stub: healthyStubQueue()}

		err := checkDelayed(t.Context(), stub, 20*time.Millisecond, 2*time.Second)
		if err == nil || !strings.Contains(err.Error(), "want later") {
			t.Fatalf("checkDelayed() = %v, want payload error", err)
		}
	})

	t.Run("early delivery", func(t *testing.T) {
		t.Parallel()

		stub := &earlyDelayedQueue{stub: healthyStubQueue()}

		err := checkDelayed(t.Context(), stub, time.Hour, time.Second)
		if err == nil || !strings.Contains(err.Error(), "before delay") {
			t.Fatalf("checkDelayed() = %v, want before-delay error", err)
		}
	})
}

// wrongDelayedQueue delivers the wrong payload after the delay.
type wrongDelayedQueue struct {
	stub *stubQueue
}

func (w *wrongDelayedQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return w.stub.Push(ctx, topic, pl, h)
}

func (w *wrongDelayedQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	if d <= 0 {
		return w.stub.PushDelayed(ctx, topic, pl, h, d)
	}
	return w.stub.PushDelayed(ctx, topic, queue.Payload("wrong"), h, d)
}

func (w *wrongDelayedQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return w.stub.Pop(ctx, topic)
}

func (w *wrongDelayedQueue) Ack(ctx context.Context, m queue.Message) error {
	return w.stub.Ack(ctx, m)
}

func (w *wrongDelayedQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return w.stub.Nack(ctx, m, r)
}

func (w *wrongDelayedQueue) Length(ctx context.Context, topic string) (int64, error) {
	return w.stub.Length(ctx, topic)
}

func (w *wrongDelayedQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return w.stub.IsEmpty(ctx, topic)
}

func (w *wrongDelayedQueue) Close() error { return w.stub.Close() }

func (w *wrongDelayedQueue) Name() string { return w.stub.Name() }

// earlyDelayedQueue delivers delayed messages immediately.
type earlyDelayedQueue struct {
	stub *stubQueue
}

func (e *earlyDelayedQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return e.stub.Push(ctx, topic, pl, h)
}

func (e *earlyDelayedQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, _ time.Duration) error {
	return e.stub.Push(ctx, topic, pl, h)
}

func (e *earlyDelayedQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return e.stub.Pop(ctx, topic)
}

func (e *earlyDelayedQueue) Ack(ctx context.Context, m queue.Message) error {
	return e.stub.Ack(ctx, m)
}

func (e *earlyDelayedQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return e.stub.Nack(ctx, m, r)
}

func (e *earlyDelayedQueue) Length(ctx context.Context, topic string) (int64, error) {
	return e.stub.Length(ctx, topic)
}

func (e *earlyDelayedQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return e.stub.IsEmpty(ctx, topic)
}

func (e *earlyDelayedQueue) Close() error { return e.stub.Close() }

func (e *earlyDelayedQueue) Name() string { return e.stub.Name() }

func TestCheckTopicsIsolatedSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkTopicsIsolated(t.Context(), healthyStubQueue()))
}

func TestCheckTopicsIsolatedFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("push a error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushErr = boom

		if err := checkTopicsIsolated(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkTopicsIsolated() = %v, want wrap of boom", err)
		}
	})

	t.Run("push b error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.pushFailOn = 2
		stub.pushFailErr = boom

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Push(b)") {
			t.Fatalf("checkTopicsIsolated() = %v, want Push(b) error", err)
		}
	})

	t.Run("pop b error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.popErrTopic = map[string]error{"topic-b": boom}

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Pop(b)") {
			t.Fatalf("checkTopicsIsolated() = %v, want Pop(b) error", err)
		}
	})

	t.Run("pop a error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.popErrTopic = map[string]error{"topic-a": boom}

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Pop(a)") {
			t.Fatalf("checkTopicsIsolated() = %v, want Pop(a) error", err)
		}
	})

	t.Run("ack a error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.ackFailOn = 1
		stub.ackFailErr = boom

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Ack(a)") {
			t.Fatalf("checkTopicsIsolated() = %v, want Ack(a) error", err)
		}
	})

	t.Run("ack b error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.ackFailOn = 2
		stub.ackFailErr = boom

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Ack(b)") {
			t.Fatalf("checkTopicsIsolated() = %v, want Ack(b) error", err)
		}
	})

	t.Run("not empty after", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.isEmptyLie = map[string]bool{"topic-a": true, "topic-b": true}

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil {
			t.Fatal("checkTopicsIsolated(lie) = nil, want errors")
		}
		for _, want := range []string{"IsEmpty(a)", "IsEmpty(b)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkTopicsIsolated() = %v, want containing %q", err, want)
			}
		}
	})

	t.Run("crossed topics", func(t *testing.T) {
		t.Parallel()

		stub := &crossedQueue{stub: healthyStubQueue()}

		err := checkTopicsIsolated(t.Context(), stub)
		if err == nil {
			t.Fatal("checkTopicsIsolated(crossed) = nil, want errors")
		}
		for _, want := range []string{"Pop(b)", "Pop(a)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkTopicsIsolated() = %v, want containing %q", err, want)
			}
		}
	})
}

// crossedQueue swaps topic payloads.
type crossedQueue struct {
	stub *stubQueue
}

func (c *crossedQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return c.stub.Push(ctx, topic, pl, h)
}

func (c *crossedQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return c.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (c *crossedQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	m, err := c.stub.Pop(ctx, topic)
	if err != nil {
		return m, err
	}
	if topic == "topic-b" {
		m.Payload = queue.Payload("a")
	} else {
		m.Payload = queue.Payload("b")
	}
	return m, nil
}

func (c *crossedQueue) Ack(ctx context.Context, m queue.Message) error {
	return c.stub.Ack(ctx, m)
}

func (c *crossedQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return c.stub.Nack(ctx, m, r)
}

func (c *crossedQueue) Length(ctx context.Context, topic string) (int64, error) {
	return c.stub.Length(ctx, topic)
}

func (c *crossedQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return c.stub.IsEmpty(ctx, topic)
}

func (c *crossedQueue) Close() error { return c.stub.Close() }

func (c *crossedQueue) Name() string { return c.stub.Name() }

func TestCheckCloseSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(t.Context(), healthyStubQueue()))
}

func TestCheckCloseFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("queuetest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubQueue()
		stub.closeErr = boom

		if err := checkClose(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("nameless and open", func(t *testing.T) {
		t.Parallel()

		stub := &namelessOpenQueue{stub: healthyStubQueue()}

		err := checkClose(t.Context(), stub)
		if err == nil {
			t.Fatal("checkClose(nameless) = nil, want errors")
		}
		for _, want := range []string{"Name() is empty", "Push()", "PushDelayed()"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkClose() = %v, want containing %q", err, want)
			}
		}
	})
}

// namelessOpenQueue has no name and ignores Close.
type namelessOpenQueue struct {
	stub *stubQueue
}

func (n *namelessOpenQueue) Push(ctx context.Context, topic string, pl queue.Payload, h queue.Headers) error {
	return n.stub.Push(ctx, topic, pl, h)
}

func (n *namelessOpenQueue) PushDelayed(ctx context.Context, topic string, pl queue.Payload, h queue.Headers, d time.Duration) error {
	return n.stub.PushDelayed(ctx, topic, pl, h, d)
}

func (n *namelessOpenQueue) Pop(ctx context.Context, topic string) (queue.Message, error) {
	return n.stub.Pop(ctx, topic)
}

func (n *namelessOpenQueue) Ack(ctx context.Context, m queue.Message) error {
	return n.stub.Ack(ctx, m)
}

func (n *namelessOpenQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return n.stub.Nack(ctx, m, r)
}

func (n *namelessOpenQueue) Length(ctx context.Context, topic string) (int64, error) {
	return n.stub.Length(ctx, topic)
}

func (n *namelessOpenQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return n.stub.IsEmpty(ctx, topic)
}

func (n *namelessOpenQueue) Close() error { return nil }

func (n *namelessOpenQueue) Name() string { return "" }

func TestPollDeliveryTimeout(t *testing.T) {
	t.Parallel()

	err := pollDelivery(t.Context(), 40*time.Millisecond, "never", func(context.Context) error {
		return queue.ErrEmpty
	})
	if err == nil || !strings.Contains(err.Error(), "condition not met") {
		t.Fatalf("pollDelivery() = %v, want timeout error", err)
	}

	mustPass(t, pollDelivery(t.Context(), time.Second, "at once", func(context.Context) error {
		return nil
	}))

	// Non-empty errors abort immediately instead of polling.
	boom := errors.New("queuetest: boom")
	if err := pollDelivery(t.Context(), time.Second, "boom", func(context.Context) error {
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("pollDelivery() = %v, want wrap of boom", err)
	}
}

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkFIFO(ctx, healthyStubQueue()); err != nil {
				t.Errorf("checkFIFO() = %v, want nil", err)
			}

			if err := checkAck(ctx, healthyStubQueue()); err != nil {
				t.Errorf("checkAck() = %v, want nil", err)
			}

			if err := checkClose(ctx, healthyStubQueue()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
