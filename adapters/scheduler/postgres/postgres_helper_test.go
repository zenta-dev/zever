package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/orm"
)

// poolWithPath builds pool options for a file-backed sqlite database.
func poolWithPath(path string) coredb.Options {
	return coredb.Options{Path: path}
}

// poolWithDSN builds pool options for a postgres database.
func poolWithDSN(dsn string) coredb.Options {
	return coredb.Options{DSN: dsn}
}

// querySlots returns every slot row in the driver's table.
func querySlots(ctx context.Context, d *driver) ([]*slotRow, error) {
	return orm.From[slotRow, *slotRow](d.tbl).All(ctx, d.conn)
}

// stubQueue is an in-memory queue.Queue recording pushes.
type stubQueue struct {
	mu     sync.Mutex
	pushes []queue.Message
}

func newStubQueue() *stubQueue { return &stubQueue{} }

func (s *stubQueue) Push(_ context.Context, _ string, payload queue.Payload, headers queue.Headers) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pushes = append(s.pushes, queue.Message{Payload: payload, Headers: headers})

	return nil
}

func (s *stubQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (s *stubQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pushes) == 0 {
		return queue.Message{}, queue.ErrEmpty
	}

	msg := s.pushes[0]
	s.pushes = s.pushes[1:]

	return msg, nil
}

func (s *stubQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (s *stubQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (s *stubQueue) Length(_ context.Context, _ string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return int64(len(s.pushes)), nil
}

func (s *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.pushes) == 0, nil
}

func (s *stubQueue) Close() error { return nil }

func (s *stubQueue) Name() string { return "kit-stub" }

// TestStubQueueShape keeps the stub honest: Push then Pop round-trips.
func TestStubQueueShape(t *testing.T) {
	t.Parallel()

	q := newStubQueue()
	ctx := t.Context()

	if err := q.Push(ctx, "low", queue.Payload(`"x"`), queue.Headers{"k": "v"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	msg, err := q.Pop(ctx, "low")
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}

	if string(msg.Payload) != `"x"` {
		t.Errorf("payload = %s, want %s", msg.Payload, `"x"`)
	}
}
