package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/queue"
)

type stubQueue struct {
	pushes      int
	lastPayload queue.Payload
}

func (s *stubQueue) Push(_ context.Context, _ string, payload queue.Payload, _ queue.Headers) error {
	s.pushes++
	s.lastPayload = payload

	return nil
}

func (s *stubQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (s *stubQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (s *stubQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (s *stubQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (s *stubQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (s *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (s *stubQueue) Close() error { return nil }

func (s *stubQueue) Name() string { return "stub" }

// TestOptionsErrorString covers InvalidOptionsError.Error, previously
// exercised through the embedded constructor test that moved with the
// implementation into scheduler/embedded.
func TestOptionsErrorString(t *testing.T) {
	t.Parallel()

	err := Options{}.Validate()
	if got, want := err.Error(), "scheduler: invalid options: dispatcher_is_required"; got != want {
		t.Fatalf("Error()=%q want %q", got, want)
	}
}
