package embedded

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
	"github.com/zenta-dev/zever/scheduler"
)

// eventually polls cond until true or timeout, failing the test on expiry.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("eventually timed out after %v: %s", timeout, msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

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

func newTestScheduler(t *testing.T) scheduler.Scheduler {
	t.Helper()

	s, err := New(scheduler.Options{Dispatcher: &job.Dispatcher{Q: &stubQueue{}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return s
}

func stubEmbedded(t *testing.T, q queue.Queue) scheduler.Scheduler {
	t.Helper()

	s, err := New(scheduler.Options{Dispatcher: &job.Dispatcher{Q: q}})
	if err != nil {
		t.Fatalf("New: %v", err)
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

	_, err := New(scheduler.Options{})
	if !errors.Is(err, scheduler.ErrInvalidOptions) {
		t.Fatalf("err=%v want ErrInvalidOptions", err)
	}
	if !strings.HasPrefix(err.Error(), "embedded: ") {
		t.Errorf("error %q missing %q prefix", err.Error(), "embedded: ")
	}
}

// TestName covers Name, previously asserted through the deleted Open
// facade test; the constructor is now the only entry point.
func TestName(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)
	if got := s.Name(); got != "embedded" {
		t.Fatalf("Name()=%q want embedded", got)
	}
}

func TestSchedule_badSpec(t *testing.T) {
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

			s := newTestScheduler(t)
			_, err := s.Schedule(t.Context(), spec, "no-such-job-ever", nil)

			if !errors.Is(err, scheduler.ErrInvalidSpec) {
				t.Fatalf("spec %q err = %v, want ErrInvalidSpec", spec, err)
			}
		})
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

			s := stubEmbedded(t, &stubQueue{})
			_, err := s.Schedule(t.Context(), spec, "no-such-job-ever", nil)

			if !errors.Is(err, scheduler.ErrInvalidSpec) {
				t.Fatalf("spec %q err=%v want ErrInvalidSpec", spec, err)
			}
		})
	}
}

func TestSchedule_specTooLong(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)
	spec := strings.Repeat("*", scheduler.MaxSpecLen+1)

	_, err := s.Schedule(t.Context(), spec, "no-such-job-ever", nil)
	if !errors.Is(err, scheduler.ErrInvalidSpec) {
		t.Fatalf("err = %v, want ErrInvalidSpec", err)
	}
}

func TestScheduleSpecTooLong(t *testing.T) {
	t.Parallel()

	s := stubEmbedded(t, &stubQueue{})
	spec := strings.Repeat("*", scheduler.MaxSpecLen+1)

	_, err := s.Schedule(t.Context(), spec, "no-such-job-ever", nil)
	if !errors.Is(err, scheduler.ErrInvalidSpec) {
		t.Fatalf("err=%v want ErrInvalidSpec", err)
	}
}

func TestSchedule_unknownJob(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

	_, err := s.Schedule(t.Context(), "0 * * * *", "emb-test-no-such-job", nil)
	if !errors.Is(err, job.ErrUnknownJob) {
		t.Fatalf("err = %v, want job.ErrUnknownJob", err)
	}
}

func TestScheduleUnknownJob(t *testing.T) {
	s := stubEmbedded(t, &stubQueue{})

	_, err := s.Schedule(t.Context(), "0 * * * *", "sched-test-no-such-job", nil)
	if !errors.Is(err, job.ErrUnknownJob) {
		t.Fatalf("err=%v want job.ErrUnknownJob", err)
	}
}

func TestSchedule_badArgs(t *testing.T) {
	registerJobOnce(t, "emb-test-badargs")

	s := newTestScheduler(t)

	_, err := s.Schedule(t.Context(), "0 * * * *", "emb-test-badargs", func() {})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScheduleBadArgs(t *testing.T) {
	registerJobOnce(t, "sched-test-badargs")

	s := stubEmbedded(t, &stubQueue{})

	_, err := s.Schedule(t.Context(), "0 * * * *", "sched-test-badargs", func() {})
	if err == nil {
		t.Fatal("unmarshalable args want error")
	}
}

func TestSchedule_ok_entries(t *testing.T) {
	registerJobOnce(t, "emb-test-ok")

	s := newTestScheduler(t)

	id, err := s.Schedule(t.Context(), "0 * * * *", "emb-test-ok", nil)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if id == 0 {
		t.Fatal("id = 0, want nonzero")
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

func TestScheduleOkEntries(t *testing.T) {
	registerJobOnce(t, "sched-test-ok")

	s := stubEmbedded(t, &stubQueue{})

	id, err := s.Schedule(t.Context(), "0 * * * *", "sched-test-ok", nil)
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

func TestSchedule_cancelledCtx(t *testing.T) {
	registerJobOnce(t, "emb-test-ctx")

	s := newTestScheduler(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.Schedule(ctx, "0 * * * *", "emb-test-ctx", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestScheduleCancelledCtx(t *testing.T) {
	registerJobOnce(t, "sched-test-ctx")

	s := stubEmbedded(t, &stubQueue{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.Schedule(ctx, "0 * * * *", "sched-test-ctx", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}

func TestRemove_zero_error(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

	if err := s.Remove(0); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveZero(t *testing.T) {
	t.Parallel()

	s := stubEmbedded(t, &stubQueue{})

	if err := s.Remove(0); err == nil {
		t.Fatal("Remove(0) want error")
	}
}

func TestRemove_unknown_noop(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

	if err := s.Remove(999999); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if got := len(s.Entries()); got != 0 {
		t.Fatalf("Entries = %d, want 0", got)
	}
}

func TestRemoveUnknownNil(t *testing.T) {
	t.Parallel()

	s := stubEmbedded(t, &stubQueue{})

	if err := s.Remove(999999); err != nil {
		t.Fatalf("Remove unknown: %v", err)
	}

	if got := len(s.Entries()); got != 0 {
		t.Fatalf("Entries=%d want 0", got)
	}
}

func TestRemove_added(t *testing.T) {
	registerJobOnce(t, "emb-test-remove")

	s := newTestScheduler(t)

	id, err := s.Schedule(t.Context(), "0 * * * *", "emb-test-remove", nil)
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

func TestRemoveAdded(t *testing.T) {
	registerJobOnce(t, "sched-test-remove")

	s := stubEmbedded(t, &stubQueue{})

	id, err := s.Schedule(t.Context(), "0 * * * *", "sched-test-remove", nil)
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

func TestStart_idempotent_stop(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

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

func TestStartIdempotent(t *testing.T) {
	t.Parallel()

	s := stubEmbedded(t, &stubQueue{})

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

func TestStop_withoutStart_nil(t *testing.T) {
	t.Parallel()

	s := newTestScheduler(t)

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStopWithoutStartNil(t *testing.T) {
	t.Parallel()

	s := stubEmbedded(t, &stubQueue{})

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

	s := stubEmbedded(t, q)

	if _, err := s.Schedule(t.Context(), "@every 1s", "sched-test-e2e", "ping"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	defer func() { _ = s.Stop() }()

	var got atomic.Bool
	// Poll for the cron tick with a second-granularity schedule instead of
	// sleeping full cron periods; the in-memory queue is the observable state.
	eventually(t, 10*time.Second, func() bool {
		// Dispatcher pushes onto the job priority topic ("low" default).
		msg, err := q.Pop(t.Context(), "low")
		if err == nil {
			if msg.Headers["job_name"] != "sched-test-e2e" {
				t.Errorf("job_name=%q want sched-test-e2e", msg.Headers["job_name"])
				return false
			}
			if string(msg.Payload) != `"ping"` {
				t.Errorf("payload=%s want %s", msg.Payload, `"ping"`)
				return false
			}
			got.Store(true)
			return true
		}
		if !errors.Is(err, queue.ErrEmpty) {
			t.Errorf("Pop: %v", err)
			return false
		}
		return false
	}, "no scheduled dispatch within 10s")
	if !got.Load() {
		t.Fatal("no scheduled dispatch within 10s")
	}
}

func TestCoverStopTimeout(t *testing.T) {
	t.Parallel()

	// runDone never closes: Stop must give up at closeTimeout.
	e := &embedded{
		cancel:       func() {},
		runDone:      make(chan struct{}),
		closeTimeout: 20 * time.Millisecond,
		started:      true,
	}

	start := time.Now()
	err := e.Stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stop took %v, want ~20ms", elapsed)
	}
}
