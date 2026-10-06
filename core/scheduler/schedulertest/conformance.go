// Package schedulertest provides the conformance kit third-party scheduler
// backends run to prove backend parity.
package schedulertest

import (
	"context"
	"errors"
	"fmt"
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
// errf builds a non-wrapping descriptive error with Sprintf semantics.
// Check helpers use it (instead of fmt.Errorf with %w) for diagnostics
// where the formatted error may be nil: %w of a nil error prints
// "%!w(<nil>)", diverging from the historical Fatalf text, and errorlint
// forbids %v of an error in Errorf. Failure text stays byte-identical.
// It deliberately avoids fmt.Errorf so only real failures wrap.
func errf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)

	return errors.New(msg)
}

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

// registerJob registers a no-op job under a process-unique name. It needs
// a real *testing.T and stays outside the error-returning checks: the job
// registry has no error-return path to unit-cover.
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

	if err := checkScheduleEntries(t.Context(), s, name); err != nil {
		t.Fatal(err)
	}
}

// checkScheduleEntries proves Schedule mints a nonzero EntryID listed by
// Entries. Job registration stays in the caller: it needs *testing.T.
func checkScheduleEntries(ctx context.Context, s scheduler.Scheduler, jobName string) error {
	id, err := s.Schedule(ctx, "0 * * * *", jobName, nil)
	if err != nil {
		return fmt.Errorf("Schedule() error = %w", err)
	}

	if id == 0 {
		return errors.New("Schedule() returned zero EntryID")
	}

	found := false

	for _, got := range s.Entries() {
		if got == id {
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("Entries() = %v, missing %v", s.Entries(), id)
	}

	if err := s.Stop(); err != nil {
		return fmt.Errorf("Stop() error = %w", err)
	}

	return nil
}

func conformanceRemove(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)
	name := registerJob(t)

	if err := checkRemove(t.Context(), s, name); err != nil {
		t.Fatal(err)
	}
}

// checkRemove proves Remove(0) errors, unknown IDs are a nil no-op, and a
// scheduled entry disappears after Remove.
func checkRemove(ctx context.Context, s scheduler.Scheduler, jobName string) error {
	if err := s.Remove(0); err == nil {
		return errors.New("Remove(0) = nil, want error")
	}

	if err := s.Remove(999999); err != nil {
		return fmt.Errorf("Remove(unknown) error = %w, want nil", err)
	}

	id, err := s.Schedule(ctx, "0 * * * *", jobName, nil)
	if err != nil {
		return fmt.Errorf("Schedule() error = %w", err)
	}

	if err := s.Remove(id); err != nil {
		return fmt.Errorf("Remove() error = %w", err)
	}

	for _, got := range s.Entries() {
		if got == id {
			return fmt.Errorf("Entries() = %v, still contains %v", s.Entries(), id)
		}
	}

	if err := s.Stop(); err != nil {
		return fmt.Errorf("Stop() error = %w", err)
	}

	return nil
}

func conformanceInvalidSpec(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)
	name := registerJob(t)

	if err := checkInvalidSpec(t.Context(), s, name); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidSpec proves malformed specs fail closed with
// ErrInvalidSpec (errors.As InvalidSpecError) and oversize specs too.
func checkInvalidSpec(ctx context.Context, s scheduler.Scheduler, jobName string) error {
	var errs []error

	for _, spec := range []string{"", "not-a-spec", "0 0 0 0 0 0", "99 * * * *", "@every", strings.Repeat("*", scheduler.MaxSpecLen+1)} {
		if _, err := s.Schedule(ctx, spec, jobName, nil); !errors.Is(err, scheduler.ErrInvalidSpec) {
			errs = append(errs, errf("Schedule(%q) err = %v, want ErrInvalidSpec", spec, err))
		}
	}

	var specErr scheduler.InvalidSpecError
	if _, err := s.Schedule(ctx, "not-a-spec", jobName, nil); !errors.As(err, &specErr) {
		errs = append(errs, errf("errors.As(err, InvalidSpecError) = false (err = %T %v)", err, err))
	}

	if err := s.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("Stop() error = %w", err))
	}

	return errors.Join(errs...)
}

func conformanceUnknownJob(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)

	if err := checkUnknownJob(t.Context(), s); err != nil {
		t.Fatal(err)
	}
}

// checkUnknownJob proves scheduling an unregistered job reports
// job.ErrUnknownJob.
func checkUnknownJob(ctx context.Context, s scheduler.Scheduler) error {
	var errs []error

	if _, err := s.Schedule(ctx, "0 * * * *", "kit-no-such-job", nil); !errors.Is(err, job.ErrUnknownJob) {
		errs = append(errs, errf("Schedule(unknown job) err = %v, want job.ErrUnknownJob", err))
	}

	if err := s.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("Stop() error = %w", err))
	}

	return errors.Join(errs...)
}

func conformanceStartStop(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	fresh, _ := kitScheduler(t, factory)
	if err := checkStopFresh(fresh); err != nil {
		t.Fatal(err)
	}

	s, _ := kitScheduler(t, factory)
	if err := checkStartStop(s); err != nil {
		t.Fatal(err)
	}
}

// checkStopFresh proves Stop without Start stays nil.
func checkStopFresh(s scheduler.Scheduler) error {
	if err := s.Stop(); err != nil {
		return fmt.Errorf("Stop() without Start error = %w, want nil", err)
	}

	return nil
}

// checkStartStop proves Start is idempotent, Stop ends ticks, and Name
// is set.
func checkStartStop(s scheduler.Scheduler) error {
	if err := s.Start(); err != nil {
		return fmt.Errorf("Start() error = %w", err)
	}

	var errs []error

	if err := s.Start(); err != nil {
		errs = append(errs, fmt.Errorf("Start() second error = %w, want nil", err))
	}

	if err := s.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("Stop() error = %w", err))
	}

	if got := s.Name(); got == "" {
		errs = append(errs, errors.New("Name() is empty"))
	}

	return errors.Join(errs...)
}

func conformanceFire(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, q := kitScheduler(t, factory)
	name := registerJob(t)

	t.Cleanup(func() { _ = s.Stop() })

	if err := checkFire(t.Context(), s, q, name, DefaultFireTimeout, DefaultPollInterval); err != nil {
		t.Fatal(err)
	}
}

// checkFire proves an @every schedule dispatches the job payload onto
// the queue within timeout. Timeout/interval are parameters so unit
// tests drive both the delivery and timeout branches.
func checkFire(ctx context.Context, s scheduler.Scheduler, q queue.Queue, jobName string, timeout, interval time.Duration) error {
	if _, err := s.Schedule(ctx, "@every 1s", jobName, "ping"); err != nil {
		return fmt.Errorf("Schedule() error = %w", err)
	}

	if err := s.Start(); err != nil {
		return fmt.Errorf("Start() error = %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		msg, err := q.Pop(ctx, "low")
		if err == nil {
			if msg.Headers["job_name"] != jobName {
				return fmt.Errorf("job_name = %q, want %q", msg.Headers["job_name"], jobName)
			}

			if string(msg.Payload) != `"ping"` {
				return fmt.Errorf("payload = %s, want %s", msg.Payload, `"ping"`)
			}

			return nil
		}

		if !errors.Is(err, queue.ErrEmpty) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return errf("Pop() error = %v", err)
		}

		select {
		case <-ctx.Done():
			return errors.New("no scheduled dispatch within timeout")
		case <-ticker.C:
		}
	}
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler) {
	t.Helper()

	s, _ := kitScheduler(t, factory)

	if err := checkOpenRegister(s); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the production Open path returns the
// registered scheduler, rejects duplicates, and reports unknown
// adapters. The dispatcher for Open uses a throwaway stub queue.
func checkOpenRegister(s scheduler.Scheduler) error {
	name := scheduler.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))

	if err := scheduler.Register(name, func(scheduler.Options) (scheduler.Scheduler, error) { return s, nil }); err != nil {
		return fmt.Errorf("Register() error = %w", err)
	}

	if err := scheduler.Register(name, func(scheduler.Options) (scheduler.Scheduler, error) { return s, nil }); !errors.Is(err, scheduler.ErrDuplicate) {
		return fmt.Errorf("Register(dup) err = %w, want ErrDuplicate", err)
	}

	opened, err := scheduler.Open(name, scheduler.Options{Dispatcher: &job.Dispatcher{Q: newStubQueue()}})
	if err != nil {
		return fmt.Errorf("Open() error = %w", err)
	}

	var errs []error

	if opened != s {
		errs = append(errs, errors.New("Open() did not return the registered scheduler"))
	}

	if _, err := scheduler.Open("kit-no-such-adapter", scheduler.Options{Dispatcher: &job.Dispatcher{Q: newStubQueue()}}); !errors.Is(err, scheduler.ErrUnknownAdapter) {
		errs = append(errs, errf("Open(unknown) err = %v, want ErrUnknownAdapter", err))
	}

	if err := s.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("Stop() error = %w", err))
	}

	return errors.Join(errs...)
}
