package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
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

func newEmbeddedForTest(t *testing.T, q queue.Queue) Scheduler {
	t.Helper()

	s, err := NewEmbedded(Options{Dispatcher: &job.Dispatcher{Q: q}})
	if err != nil {
		t.Fatalf("NewEmbedded: %v", err)
	}

	return s
}

func registerJobOnce(t *testing.T, name string) {
	t.Helper()

	if err := job.Register(name, func(context.Context, string) error { return nil }); err != nil {
		var dup *job.DuplicateJobError
		if !errors.As(err, &dup) {
			t.Fatalf("Register: %v", err)
		}
	}
}

func TestNewEmbeddedNilDispatcher(t *testing.T) {
	t.Parallel()

	_, err := NewEmbedded(Options{})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
	if !strings.HasPrefix(err.Error(), "embedded: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "embedded: ")
	}
}

func TestScheduleBadSpec(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"empty":     "",
		"garbage":   "not-a-spec",
		"sixfields": "0 0 0 0 0 0",
		"badrange":  "99 * * * *",
		"bareevery": "@every",
	}

	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := newEmbeddedForTest(t, &stubQueue{})
			_, err := s.Schedule(context.Background(), spec, "no-such-job-ever", nil)

			if !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("spec %q err=%v want ErrInvalidSpec", spec, err)
			}
		})
	}
}

func TestScheduleSpecTooLong(t *testing.T) {
	t.Parallel()

	s := newEmbeddedForTest(t, &stubQueue{})
	spec := strings.Repeat("*", MaxSpecLen+1)

	_, err := s.Schedule(context.Background(), spec, "no-such-job-ever", nil)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("err=%v want ErrInvalidSpec", err)
	}
}

func TestScheduleUnknownJob(t *testing.T) {
	s := newEmbeddedForTest(t, &stubQueue{})

	_, err := s.Schedule(context.Background(), "0 * * * *", "sched-test-no-such-job", nil)
	if !errors.Is(err, job.ErrUnknownJob) {
		t.Fatalf("err=%v want job.ErrUnknownJob", err)
	}
}

func TestScheduleBadArgs(t *testing.T) {
	registerJobOnce(t, "sched-test-badargs")

	s := newEmbeddedForTest(t, &stubQueue{})

	_, err := s.Schedule(context.Background(), "0 * * * *", "sched-test-badargs", func() {})
	if err == nil {
		t.Fatal("unmarshalable args want error")
	}
}

func TestScheduleOkEntries(t *testing.T) {
	registerJobOnce(t, "sched-test-ok")

	s := newEmbeddedForTest(t, &stubQueue{})

	id, err := s.Schedule(context.Background(), "0 * * * *", "sched-test-ok", nil)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if id == 0 {
		t.Fatal("id=0 want nonzero")
	}

	found := false

	for _, got := range s.Entries() {
		if got == id {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("Entries %v missing id %v", s.Entries(), id)
	}
}

func TestScheduleCancelledCtx(t *testing.T) {
	registerJobOnce(t, "sched-test-ctx")

	s := newEmbeddedForTest(t, &stubQueue{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Schedule(ctx, "0 * * * *", "sched-test-ctx", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}

func TestRemoveZero(t *testing.T) {
	t.Parallel()

	s := newEmbeddedForTest(t, &stubQueue{})

	if err := s.Remove(0); err == nil {
		t.Fatal("Remove(0) want error")
	}
}

func TestRemoveUnknownNil(t *testing.T) {
	t.Parallel()

	s := newEmbeddedForTest(t, &stubQueue{})

	if err := s.Remove(999999); err != nil {
		t.Fatalf("Remove unknown: %v", err)
	}

	if got := len(s.Entries()); got != 0 {
		t.Fatalf("Entries=%d want 0", got)
	}
}

func TestRemoveAdded(t *testing.T) {
	registerJobOnce(t, "sched-test-remove")

	s := newEmbeddedForTest(t, &stubQueue{})

	id, err := s.Schedule(context.Background(), "0 * * * *", "sched-test-remove", nil)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	for _, got := range s.Entries() {
		if got == id {
			t.Fatalf("Entries %v still contains %v", s.Entries(), id)
		}
	}
}

func TestStartIdempotent(t *testing.T) {
	t.Parallel()

	s := newEmbeddedForTest(t, &stubQueue{})

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStopWithoutStartNil(t *testing.T) {
	t.Parallel()

	s := newEmbeddedForTest(t, &stubQueue{})

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop without Start: %v", err)
	}
}

func TestFireEndToEnd(t *testing.T) {
	registerJobOnce(t, "sched-test-e2e")

	q, err := queuememory.New(queue.Options{})
	if err != nil {
		t.Fatalf("queuememory.New: %v", err)
	}

	defer q.Close()

	s := newEmbeddedForTest(t, q)

	if _, err := s.Schedule(context.Background(), "@every 1s", "sched-test-e2e", "ping"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	defer func() { _ = s.Stop() }()

	deadline := time.Now().Add(10 * time.Second)

	for {
		// Dispatcher pushes onto the job priority topic ("low" default).
		msg, err := q.Pop(context.Background(), "low")
		if err == nil {
			if msg.Headers["job_name"] != "sched-test-e2e" {
				t.Fatalf("job_name=%q want sched-test-e2e", msg.Headers["job_name"])
			}

			if string(msg.Payload) != `"ping"` {
				t.Fatalf("payload=%s want %s", msg.Payload, `"ping"`)
			}

			return
		}

		if !errors.Is(err, queue.ErrEmpty) {
			t.Fatalf("Pop: %v", err)
		}

		if time.Now().After(deadline) {
			t.Fatal("no scheduled dispatch within 10s")
		}

		time.Sleep(100 * time.Millisecond)
	}
}
