package schedulertest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// stubScheduler is a scriptable scheduler.Scheduler double with a small
// entry map. Per-method error fields drive each conformance failure
// branch; fire hooks simulate dispatch onto the stub queue.
type stubScheduler struct {
	mu sync.Mutex

	entries     map[scheduler.EntryID]string
	next        scheduler.EntryID
	schedErr    error
	removeErr   error
	removeKept  bool
	entriesDrop bool
	startErr    error
	startAg     error
	starts      int
	stopErr     error
	stops       int
	stopped     bool
	name        string
	// fire pushes a dispatch onto q when Start runs.
	fire        bool
	fireQ       *stubQueue
	firePayload queue.Payload
	fireHeaders queue.Headers
}

func healthyStubScheduler() *stubScheduler {
	return &stubScheduler{entries: map[scheduler.EntryID]string{}, next: 1, name: "stub"}
}

func (s *stubScheduler) Schedule(_ context.Context, spec, jobName string, _ any) (scheduler.EntryID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.schedErr != nil {
		return 0, s.schedErr
	}
	if spec == "" || spec == "not-a-spec" || spec == "0 0 0 0 0 0" || spec == "99 * * * *" || spec == "@every" || len(spec) > scheduler.MaxSpecLen {
		return 0, scheduler.InvalidSpecError{Spec: spec}
	}
	if _, ok := job.Lookup(jobName); !ok {
		return 0, job.ErrUnknownJob
	}
	id := s.next
	s.next++
	s.entries[id] = jobName
	return id, nil
}

func (s *stubScheduler) Remove(id scheduler.EntryID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.removeErr != nil && id == 0 {
		return s.removeErr
	}
	if id == 0 {
		return scheduler.InvalidSpecError{Spec: "zero entry"}
	}
	if !s.removeKept {
		delete(s.entries, id)
	}
	return nil
}

func (s *stubScheduler) Entries() []scheduler.EntryID {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.entriesDrop {
		return nil
	}
	out := make([]scheduler.EntryID, 0, len(s.entries))
	for id := range s.entries {
		out = append(out, id)
	}
	return out
}

func (s *stubScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.starts++
	if s.starts > 1 {
		return s.startAg
	}
	if s.startErr != nil {
		return s.startErr
	}
	if s.fire && s.fireQ != nil {
		s.fireQ.mu.Lock()
		s.fireQ.pushes = append(s.fireQ.pushes, queue.Message{Payload: s.firePayload, Headers: s.fireHeaders})
		s.fireQ.mu.Unlock()
	}
	return nil
}

func (s *stubScheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stops++
	if s.stopErr != nil {
		return s.stopErr
	}
	s.stopped = true
	return nil
}

func (s *stubScheduler) Name() string {
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

func TestCheckScheduleEntriesSuccess(t *testing.T) {
	t.Parallel()

	name := registerJob(t)
	mustPass(t, checkScheduleEntries(t.Context(), healthyStubScheduler(), name))
}

func TestCheckScheduleEntriesFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("schedulertest: boom")

	t.Run("schedule error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.schedErr = boom

		if err := checkScheduleEntries(t.Context(), stub, name); !errors.Is(err, boom) {
			t.Fatalf("checkScheduleEntries() = %v, want wrap of boom", err)
		}
	})

	t.Run("zero id", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := &zeroIDScheduler{stub: healthyStubScheduler()}

		err := checkScheduleEntries(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "zero EntryID") {
			t.Fatalf("checkScheduleEntries() = %v, want zero-ID error", err)
		}
	})

	t.Run("missing from entries", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.entriesDrop = true

		err := checkScheduleEntries(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("checkScheduleEntries() = %v, want missing error", err)
		}
	})

	t.Run("stop error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.stopErr = boom

		err := checkScheduleEntries(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "Stop() error") {
			t.Fatalf("checkScheduleEntries() = %v, want Stop error", err)
		}
	})
}

// zeroIDScheduler mints zero EntryIDs.
type zeroIDScheduler struct {
	stub *stubScheduler
}

func (z *zeroIDScheduler) Schedule(context.Context, string, string, any) (scheduler.EntryID, error) {
	return 0, nil
}

func (z *zeroIDScheduler) Remove(id scheduler.EntryID) error { return z.stub.Remove(id) }

func (z *zeroIDScheduler) Entries() []scheduler.EntryID { return z.stub.Entries() }

func (z *zeroIDScheduler) Start() error { return z.stub.Start() }

func (z *zeroIDScheduler) Stop() error { return z.stub.Stop() }

func (z *zeroIDScheduler) Name() string { return z.stub.Name() }

func TestCheckRemoveSuccess(t *testing.T) {
	t.Parallel()

	name := registerJob(t)
	mustPass(t, checkRemove(t.Context(), healthyStubScheduler(), name))
}

func TestCheckRemoveFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("schedulertest: boom")

	t.Run("remove zero ok", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := &tolerantRemoveScheduler{stub: healthyStubScheduler()}

		err := checkRemove(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "Remove(0)") {
			t.Fatalf("checkRemove() = %v, want Remove(0) error", err)
		}
	})

	t.Run("unknown errors", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := &failUnknownRemoveScheduler{stub: healthyStubScheduler(), err: boom}

		err := checkRemove(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "Remove(unknown)") {
			t.Fatalf("checkRemove() = %v, want unknown error", err)
		}
	})

	t.Run("schedule error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.schedErr = boom

		if err := checkRemove(t.Context(), stub, name); !errors.Is(err, boom) {
			t.Fatalf("checkRemove() = %v, want wrap of boom", err)
		}
	})

	t.Run("entry kept", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.removeKept = true

		err := checkRemove(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "still contains") {
			t.Fatalf("checkRemove() = %v, want still-contains error", err)
		}
	})
}

// tolerantRemoveScheduler accepts Remove(0).
type tolerantRemoveScheduler struct {
	stub *stubScheduler
}

func (t2 *tolerantRemoveScheduler) Schedule(ctx context.Context, spec, name string, args any) (scheduler.EntryID, error) {
	return t2.stub.Schedule(ctx, spec, name, args)
}

func (t2 *tolerantRemoveScheduler) Remove(scheduler.EntryID) error { return nil }

func (t2 *tolerantRemoveScheduler) Entries() []scheduler.EntryID { return t2.stub.Entries() }

func (t2 *tolerantRemoveScheduler) Start() error { return t2.stub.Start() }

func (t2 *tolerantRemoveScheduler) Stop() error { return t2.stub.Stop() }

func (t2 *tolerantRemoveScheduler) Name() string { return t2.stub.Name() }

// failUnknownRemoveScheduler fails unknown-ID removes.
type failUnknownRemoveScheduler struct {
	stub *stubScheduler
	err  error
}

func (f *failUnknownRemoveScheduler) Schedule(ctx context.Context, spec, name string, args any) (scheduler.EntryID, error) {
	return f.stub.Schedule(ctx, spec, name, args)
}

func (f *failUnknownRemoveScheduler) Remove(id scheduler.EntryID) error {
	if id == 999999 {
		return f.err
	}
	return f.stub.Remove(id)
}

func (f *failUnknownRemoveScheduler) Entries() []scheduler.EntryID { return f.stub.Entries() }

func (f *failUnknownRemoveScheduler) Start() error { return f.stub.Start() }

func (f *failUnknownRemoveScheduler) Stop() error { return f.stub.Stop() }

func (f *failUnknownRemoveScheduler) Name() string { return f.stub.Name() }

func TestCheckInvalidSpecSuccess(t *testing.T) {
	t.Parallel()

	name := registerJob(t)
	mustPass(t, checkInvalidSpec(t.Context(), healthyStubScheduler(), name))
}

func TestCheckInvalidSpecFailures(t *testing.T) {
	t.Parallel()

	t.Run("accepts everything", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := &acceptAllScheduler{stub: healthyStubScheduler()}

		err := checkInvalidSpec(t.Context(), stub, name)
		if err == nil {
			t.Fatal("checkInvalidSpec(accept-all) = nil, want joined errors")
		}
		for _, want := range []string{"ErrInvalidSpec", "InvalidSpecError"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkInvalidSpec() = %v, want containing %q", err, want)
			}
		}
	})

	t.Run("stop error joins", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.stopErr = errors.New("schedulertest: stop boom")

		err := checkInvalidSpec(t.Context(), stub, name)
		if err == nil || !strings.Contains(err.Error(), "Stop() error") {
			t.Fatalf("checkInvalidSpec() = %v, want Stop error", err)
		}
	})
}

// acceptAllScheduler schedules any spec without validation.
type acceptAllScheduler struct {
	stub *stubScheduler
}

func (a *acceptAllScheduler) Schedule(_ context.Context, _, _ string, _ any) (scheduler.EntryID, error) {
	a.stub.mu.Lock()
	defer a.stub.mu.Unlock()

	id := a.stub.next
	a.stub.next++
	return id, nil
}

func (a *acceptAllScheduler) Remove(id scheduler.EntryID) error { return a.stub.Remove(id) }

func (a *acceptAllScheduler) Entries() []scheduler.EntryID { return a.stub.Entries() }

func (a *acceptAllScheduler) Start() error { return a.stub.Start() }

func (a *acceptAllScheduler) Stop() error { return a.stub.Stop() }

func (a *acceptAllScheduler) Name() string { return a.stub.Name() }

func TestCheckUnknownJobSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkUnknownJob(t.Context(), healthyStubScheduler()))
}

func TestCheckUnknownJobFailures(t *testing.T) {
	t.Parallel()

	t.Run("accepts unknown", func(t *testing.T) {
		t.Parallel()

		stub := &acceptAllScheduler{stub: healthyStubScheduler()}

		err := checkUnknownJob(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "unknown job") {
			t.Fatalf("checkUnknownJob() = %v, want unknown-job error", err)
		}
	})
}

func TestCheckStartStopSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkStopFresh(healthyStubScheduler()))
	mustPass(t, checkStartStop(healthyStubScheduler()))
}

func TestCheckStartStopFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("schedulertest: boom")

	t.Run("stop fresh error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubScheduler()
		stub.stopErr = boom

		if err := checkStopFresh(stub); !errors.Is(err, boom) {
			t.Fatalf("checkStopFresh() = %v, want wrap of boom", err)
		}
	})

	t.Run("start error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubScheduler()
		stub.startErr = boom

		if err := checkStartStop(stub); !errors.Is(err, boom) {
			t.Fatalf("checkStartStop() = %v, want wrap of boom", err)
		}
	})

	t.Run("second start and stop and name", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubScheduler()
		stub.startAg = boom
		stub.stopErr = boom
		stub.name = ""

		err := checkStartStop(stub)
		if err == nil {
			t.Fatal("checkStartStop() = nil, want joined errors")
		}
		for _, want := range []string{"second", "Stop()", "Name()"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkStartStop() = %v, want containing %q", err, want)
			}
		}
	})
}

func TestCheckFireSuccess(t *testing.T) {
	t.Parallel()

	name := registerJob(t)
	q := newStubQueue()
	stub := healthyStubScheduler()
	stub.fire = true
	stub.fireQ = q
	stub.firePayload = queue.Payload(`"ping"`)
	stub.fireHeaders = queue.Headers{"job_name": name}

	mustPass(t, checkFire(t.Context(), stub, q, name, 2*time.Second, 5*time.Millisecond))
}

func TestCheckFireFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("schedulertest: boom")

	t.Run("schedule error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.schedErr = boom

		if err := checkFire(t.Context(), stub, newStubQueue(), name, time.Second, 5*time.Millisecond); !errors.Is(err, boom) {
			t.Fatalf("checkFire() = %v, want wrap of boom", err)
		}
	})

	t.Run("start error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.startErr = boom

		if err := checkFire(t.Context(), stub, newStubQueue(), name, time.Second, 5*time.Millisecond); !errors.Is(err, boom) {
			t.Fatalf("checkFire() = %v, want wrap of boom", err)
		}
	})

	t.Run("never fires", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()

		err := checkFire(t.Context(), stub, newStubQueue(), name, 80*time.Millisecond, 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "no scheduled dispatch") {
			t.Fatalf("checkFire() = %v, want dispatch-timeout error", err)
		}
	})

	t.Run("wrong job name", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		q := newStubQueue()
		stub := healthyStubScheduler()
		stub.fire = true
		stub.fireQ = q
		stub.firePayload = queue.Payload(`"ping"`)
		stub.fireHeaders = queue.Headers{"job_name": "other"}

		err := checkFire(t.Context(), stub, q, name, 2*time.Second, 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "job_name") {
			t.Fatalf("checkFire() = %v, want job_name error", err)
		}
	})

	t.Run("wrong payload", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		q := newStubQueue()
		stub := healthyStubScheduler()
		stub.fire = true
		stub.fireQ = q
		stub.firePayload = queue.Payload(`"pong"`)
		stub.fireHeaders = queue.Headers{"job_name": name}

		err := checkFire(t.Context(), stub, q, name, 2*time.Second, 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "payload") {
			t.Fatalf("checkFire() = %v, want payload error", err)
		}
	})

	t.Run("pop error", func(t *testing.T) {
		t.Parallel()

		name := registerJob(t)
		stub := healthyStubScheduler()
		stub.fire = true
		stub.fireQ = newStubQueue()
		stub.firePayload = queue.Payload(`"ping"`)
		stub.fireHeaders = queue.Headers{"job_name": name}

		err := checkFire(t.Context(), stub, &errQueue{q: newStubQueue(), err: boom}, name, 2*time.Second, 5*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "Pop() error") {
			t.Fatalf("checkFire() = %v, want Pop error", err)
		}
	})
}

// errQueue fails every Pop with err.
type errQueue struct {
	q   *stubQueue
	err error
}

func (e *errQueue) Push(ctx context.Context, topic string, p queue.Payload, h queue.Headers) error {
	return e.q.Push(ctx, topic, p, h)
}

func (e *errQueue) PushDelayed(ctx context.Context, topic string, p queue.Payload, h queue.Headers, d time.Duration) error {
	return e.q.PushDelayed(ctx, topic, p, h, d)
}

func (e *errQueue) Pop(context.Context, string) (queue.Message, error) {
	return queue.Message{}, e.err
}

func (e *errQueue) Ack(ctx context.Context, m queue.Message) error { return e.q.Ack(ctx, m) }

func (e *errQueue) Nack(ctx context.Context, m queue.Message, r bool) error {
	return e.q.Nack(ctx, m, r)
}

func (e *errQueue) Length(ctx context.Context, topic string) (int64, error) {
	return e.q.Length(ctx, topic)
}

func (e *errQueue) IsEmpty(ctx context.Context, topic string) (bool, error) {
	return e.q.IsEmpty(ctx, topic)
}

func (e *errQueue) Close() error { return e.q.Close() }

func (e *errQueue) Name() string { return e.q.Name() }

func TestCheckOpenRegisterSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister(healthyStubScheduler()))
}

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	name := registerJob(t)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkScheduleEntries(ctx, healthyStubScheduler(), name); err != nil {
				t.Errorf("checkScheduleEntries() = %v, want nil", err)
			}

			if err := checkStartStop(healthyStubScheduler()); err != nil {
				t.Errorf("checkStartStop() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
