// Package jobtest provides the conformance kit for the job dispatcher-over-Queue contract.
//
// Job owns no adapter registry: Dispatcher enqueues registered jobs
// onto a queue.Queue and Worker executes them. The kit is
// contract-shaped rather than factory-shaped: Conformance takes no
// factory and wires the in-memory queue plus cache adapters the
// core/job module already requires, so dispatcher, uniqueness, and
// worker-execution semantics stay pinned without live infra.
package jobtest

import (
	"context"
	"errors"
	"testing"
	"time"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
)

// DefaultSettleTimeout bounds how long worker-execution probes wait
// for a handler to run before failing.
const DefaultSettleTimeout = 2 * time.Second

// Conformance verifies the job contract: register validation,
// unknown-job dispatch sentinel, dispatch-to-queue round-trip with
// worker execution, UniqueBy dedup, and past-time scheduling. Tests
// never call time.Sleep and never touch the network; worker waits
// block on handler channels with a timeout, never sleeps.
//
// The kit resets the global job registry per subtest and never runs
// subtests in parallel: job definitions are process-global.
func Conformance(t *testing.T) {
	t.Helper()

	t.Run("RegisterValidate", conformanceRegisterValidate)
	t.Run("DispatchUnknown", conformanceDispatchUnknown)
	t.Run("DispatchRoundTrip", conformanceDispatchRoundTrip)
	t.Run("UniqueDedup", conformanceUniqueDedup)
	t.Run("PastAtImmediate", conformancePastAtImmediate)
}

// isolate resets the global registry before and after each case.
func isolate(t *testing.T) {
	t.Helper()

	job.Reset()
	t.Cleanup(job.Reset)
}

// freshQueue returns a memory queue with short poll timeouts.
func freshQueue(t *testing.T) queue.Queue {
	t.Helper()

	q, err := queuememory.New(queue.Options{
		Buffer:            128,
		PollTimeout:       20 * time.Millisecond,
		VisibilityTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New queue error = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	return q
}

// freshCache returns a memory cache backing uniqueness locks.
func freshCache(t *testing.T) cache.Cache {
	t.Helper()

	c, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("New cache error = %v", err)
	}

	t.Cleanup(func() { _ = c.Close(t.Context()) })

	return c
}

func conformanceRegisterValidate(t *testing.T) {
	t.Helper()
	isolate(t)

	if err := job.Register("", func(_ context.Context, _ struct{}) error { return nil }); !errors.Is(err, job.ErrRegisterNameEmpty) {
		t.Errorf("Register(empty) err = %v, want ErrRegisterNameEmpty", err)
	}

	if err := job.Register[struct{}]("kit.nil", nil); !errors.Is(err, job.ErrRegisterHandleNil) {
		t.Errorf("Register(nil) err = %v, want ErrRegisterHandleNil", err)
	}

	if err := job.Register("kit.dup", func(_ context.Context, _ struct{}) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := job.Register("kit.dup", func(_ context.Context, _ struct{}) error { return nil }); !errors.Is(err, job.ErrDuplicateJob) {
		t.Errorf("Register(duplicate) err = %v, want ErrDuplicateJob", err)
	}

	var dupErr *job.DuplicateJobError
	if err := job.Register("kit.dup", func(_ context.Context, _ struct{}) error { return nil }); !errors.As(err, &dupErr) {
		t.Errorf("errors.As(err, DuplicateJobError) = false (err = %T %v)", err, err)
	}

	if err := job.Use(nil); !errors.Is(err, job.ErrUseMiddlewareNil) {
		t.Errorf("Use(nil) err = %v, want ErrUseMiddlewareNil", err)
	}

	if _, ok := job.Lookup("kit.missing"); ok {
		t.Error("Lookup(missing) = true, want false")
	}
}

func conformanceDispatchUnknown(t *testing.T) {
	t.Helper()
	isolate(t)

	d := &job.Dispatcher{Q: freshQueue(t)}

	if err := d.Dispatch(t.Context(), "kit.unknown", struct{}{}); !errors.Is(err, job.ErrUnknownJob) {
		t.Errorf("Dispatch(unknown) err = %v, want ErrUnknownJob", err)
	}

	var unknownErr *job.UnknownJobError
	if err := d.Dispatch(t.Context(), "kit.unknown", struct{}{}); !errors.As(err, &unknownErr) {
		t.Errorf("errors.As(err, UnknownJobError) = false (err = %T %v)", err, err)
	}
}

func conformanceDispatchRoundTrip(t *testing.T) {
	t.Helper()
	isolate(t)

	ctx := t.Context()
	q := freshQueue(t)
	d := &job.Dispatcher{Q: q}

	type kitArgs struct {
		Name string `json:"name"`
	}

	done := make(chan string, 1)

	if err := job.Register("kit.echo", func(_ context.Context, args kitArgs) error {
		done <- args.Name
		return nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := d.Dispatch(ctx, "kit.echo", kitArgs{Name: "kit"}); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	if n, err := q.Length(ctx, "low"); err != nil || n != 1 {
		t.Fatalf("Length(low) = %d,%v want 1,nil", n, err)
	}

	msg, err := q.Pop(ctx, "low")
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}

	if msg.Headers["job_name"] != "kit.echo" {
		t.Errorf("header job_name = %q, want kit.echo", msg.Headers["job_name"])
	}

	if err := q.Ack(ctx, msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	// Re-dispatch through a worker to prove end-to-end execution.
	if err := d.Dispatch(ctx, "kit.echo", kitArgs{Name: "kit"}); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	wctx, cancel := context.WithTimeout(ctx, DefaultSettleTimeout)
	defer cancel()

	w := &job.Worker{Q: q, Queues: []string{"low"}}

	doneRun := make(chan error, 1)
	go func() { doneRun <- w.Run(wctx) }()

	select {
	case got := <-done:
		if got != "kit" {
			t.Errorf("handler args.Name = %q, want kit", got)
		}
	case <-wctx.Done():
		t.Fatal("handler did not run within timeout")
	}

	cancel()

	if err := <-doneRun; err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want nil or Canceled", err)
	}
}

func conformanceUniqueDedup(t *testing.T) {
	t.Helper()
	isolate(t)

	ctx := t.Context()
	q := freshQueue(t)
	d := &job.Dispatcher{Q: q, UniqueLocker: job.NewUniqueLocker(freshCache(t))}

	if err := job.Register("kit.unique", func(_ context.Context, _ struct{}) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := d.Dispatch(ctx, "kit.unique", struct{}{}, job.UniqueBy("kit-key")); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	if err := d.Dispatch(ctx, "kit.unique", struct{}{}, job.UniqueBy("kit-key")); err != nil {
		t.Fatalf("Dispatch(duplicate) error = %v, want nil skip", err)
	}

	if n, err := q.Length(ctx, "low"); err != nil || n != 1 {
		t.Errorf("Length(low) = %d,%v want 1 (deduped)", n, err)
	}
}

func conformancePastAtImmediate(t *testing.T) {
	t.Helper()
	isolate(t)

	ctx := t.Context()
	q := freshQueue(t)
	d := &job.Dispatcher{Q: q}

	if err := job.Register("kit.past", func(_ context.Context, _ struct{}) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := d.Dispatch(ctx, "kit.past", struct{}{}, job.At(time.Now().Add(-time.Minute))); err != nil {
		t.Fatalf("Dispatch(past At) error = %v", err)
	}

	if n, err := q.Length(ctx, "low"); err != nil || n != 1 {
		t.Errorf("Length(low) = %d,%v want 1 (past dispatches immediately)", n, err)
	}
}
