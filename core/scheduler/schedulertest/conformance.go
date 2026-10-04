// Package schedulertest provides the conformance kit third-party scheduler
// backends run to prove backend parity.
package schedulertest

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

const (
	// DefaultFireTimeout bounds how long fire-polling waits for a tick.
	DefaultFireTimeout = 10 * time.Second
	// DefaultPollInterval is the tick between fire-poll attempts.
	DefaultPollInterval = 20 * time.Millisecond
)

// adapterSeq keeps throwaway registry names unique across Conformance runs
// in one process. jobSeq keeps globally registered job names unique so
// parallel adapter suites never collide on the process-wide job registry.
var (
	adapterSeq atomic.Uint64
	jobSeq     atomic.Uint64
)

// Conformance verifies factory-built schedulers implement the
// scheduler.Scheduler contract against the dispatcher it is given:
// Schedule/Entries/Remove lifecycle, spec and job validation, idempotent
// Start, Stop, Open/Register wiring, and at-least-once in-process firing.
// Each subtest takes a fresh instance from factory so cases stay isolated.
// Fire-polling uses a ticker with a deadline, never time.Sleep, and never
// touches the network.
//
// Single-fire-in-process contract: the kit proves one instance fires each
// slot at least once. It does not prove cross-instance mutual exclusion:
// the embedded adapter double-fires when two instances share a queue, so
// multi-instance deploys need the durable leased postgres adapter, which
// claims each slot in the database before dispatching and skips slots held
// by a live owner.
//
// Factory receives a kit-owned Dispatcher pushing onto a kit-owned stub
// queue: adapters must honor opts.Dispatcher (and opts.Locker when set)
// rather than opening their own queue, so container.Job wiring stays the
// single queue connection.
func Conformance(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	t.Run("ScheduleEntries", func(t *testing.T) { conformanceScheduleEntries(t, factory) })
	t.Run("Remove", func(t *testing.T) { conformanceRemove(t, factory) })
	t.Run("InvalidSpec", func(t *testing.T) { conformanceInvalidSpec(t, factory) })
	t.Run("UnknownJob", func(t *testing.T) { conformanceUnknownJob(t, factory) })
	t.Run("StartStop", func(t *testing.T) { conformanceStartStop(t, factory) })
	t.Run("Fire", func(t *testing.T) { conformanceFire(t, factory) })
	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t, factory) })
}

// stubQueue is an in-memory queue.Queue recording pushes for fire polling.
type stubQueue struct {
	mu       sync.Mutex
	pushes   []queue.Message
	pushCh   chan struct{}
	jobTopic string
}

func newStubQueue() *stubQueue {
	return &stubQueue{pushCh: make(chan struct{}, 1024)}
}

func (s *stubQueue) Push(_ context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pushes = append(s.pushes, queue.Message{Payload: payload, Headers: headers})
	s.jobTopic = topic

	select {
	case s.pushCh <- struct{}{}:
	default:
	}

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

// kitScheduler builds a kit-wired scheduler: the stub queue stays visible
// to the test for fire polling while factory owns scheduler construction.
func kitScheduler(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) (scheduler.Scheduler, *stubQueue) {
	t.Helper()

	q := newStubQueue()
	s := factory(t, &job.Dispatcher{Q: q})

	return s, q
}

// registerJob registers a no-op job under a process-unique name.
func registerJob(t *testing.T) string {
	t.Helper()

	name := "kit-job-" + strconv.FormatUint(jobSeq.Add(1), 10)

	if err := job.Register(name, func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("job.Register() error = %v", err)
	}

	return name
}

func conformanceScheduleEntries(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)
	name := registerJob(t)

	id, err := s.Schedule(t.Context(), "0 * * * *", name, nil)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	if id == 0 {
		t.Fatal("Schedule() returned zero EntryID")
	}

	found := false

	for _, got := range s.Entries() {
		if got == id {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("Entries() = %v, missing %v", s.Entries(), id)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func conformanceRemove(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)

	if err := s.Remove(0); err == nil {
		t.Error("Remove(0) = nil, want error")
	}

	if err := s.Remove(999999); err != nil {
		t.Errorf("Remove(unknown) error = %v, want nil", err)
	}

	name := registerJob(t)

	id, err := s.Schedule(t.Context(), "0 * * * *", name, nil)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	for _, got := range s.Entries() {
		if got == id {
			t.Errorf("Entries() = %v, still contains %v", s.Entries(), id)
		}
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func conformanceInvalidSpec(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)
	name := registerJob(t)
	ctx := t.Context()

	for _, spec := range []string{"", "not-a-spec", "0 0 0 0 0 0", "99 * * * *", "@every", strings.Repeat("*", scheduler.MaxSpecLen+1)} {
		if _, err := s.Schedule(ctx, spec, name, nil); !errors.Is(err, scheduler.ErrInvalidSpec) {
			t.Errorf("Schedule(%q) err = %v, want ErrInvalidSpec", spec, err)
		}
	}

	var specErr scheduler.InvalidSpecError
	if _, err := s.Schedule(ctx, "not-a-spec", name, nil); !errors.As(err, &specErr) {
		t.Errorf("errors.As(err, InvalidSpecError) = false (err = %T %v)", err, err)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func conformanceUnknownJob(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)

	if _, err := s.Schedule(t.Context(), "0 * * * *", "kit-no-such-job", nil); !errors.Is(err, job.ErrUnknownJob) {
		t.Errorf("Schedule(unknown job) err = %v, want job.ErrUnknownJob", err)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func conformanceStartStop(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	fresh, _ := kitScheduler(t, factory)
	if err := fresh.Stop(); err != nil {
		t.Errorf("Stop() without Start error = %v, want nil", err)
	}

	s, _ := kitScheduler(t, factory)

	if err := s.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := s.Start(); err != nil {
		t.Errorf("Start() second error = %v, want nil", err)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if got := s.Name(); got == "" {
		t.Error("Name() is empty")
	}
}

func conformanceFire(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, q := kitScheduler(t, factory)
	name := registerJob(t)

	if _, err := s.Schedule(t.Context(), "@every 1s", name, "ping"); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Stop() })

	ctx, cancel := context.WithTimeout(t.Context(), DefaultFireTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		msg, err := q.Pop(ctx, "low")
		if err == nil {
			if msg.Headers["job_name"] != name {
				t.Fatalf("job_name = %q, want %q", msg.Headers["job_name"], name)
			}

			if string(msg.Payload) != `"ping"` {
				t.Fatalf("payload = %s, want %s", msg.Payload, `"ping"`)
			}

			return
		}

		if !errors.Is(err, queue.ErrEmpty) {
			t.Fatalf("Pop() error = %v", err)
		}

		select {
		case <-ctx.Done():
			t.Fatal("no scheduled dispatch within timeout")
		case <-ticker.C:
		}
	}
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)
	name := scheduler.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))

	if err := scheduler.Register(name, func(scheduler.Options) (scheduler.Scheduler, error) { return s, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := scheduler.Register(name, func(scheduler.Options) (scheduler.Scheduler, error) { return s, nil }); !errors.Is(err, scheduler.ErrDuplicate) {
		t.Fatalf("Register(dup) err = %v, want ErrDuplicate", err)
	}

	opened, err := scheduler.Open(name, scheduler.Options{Dispatcher: &job.Dispatcher{Q: newStubQueue()}})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if opened != s {
		t.Error("Open() did not return the registered scheduler")
	}

	if _, err := scheduler.Open("kit-no-such-adapter", scheduler.Options{Dispatcher: &job.Dispatcher{Q: newStubQueue()}}); !errors.Is(err, scheduler.ErrUnknownAdapter) {
		t.Errorf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}
