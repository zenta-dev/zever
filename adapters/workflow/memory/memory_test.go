package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
)

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()

	w, err := New(workflow.Options{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	a, ok := w.(*Adapter)
	if !ok {
		t.Fatalf("New returned %T, want *Adapter", w)
	}

	return a
}

func echoStep(_ context.Context, input any) (any, error) {
	return input, nil
}

func TestSignalAfterCompletionRejected(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	runID, err := a.Start(ctx, "step1", "hello", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if runID == "" {
		t.Fatal("expected non-empty RunID")
	}

	err = a.Signal(ctx, runID, "advance", "world")
	if !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Signal = %v, want ErrRunCompleted", err)
	}

	var result string

	err = a.Query(ctx, runID, "state", &result)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result != "hello" {
		t.Fatalf("expected state %q unchanged, got %q", "hello", result)
	}

	_ = a.Close()
}

func TestCancel(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)

	started := make(chan struct{})
	release := make(chan struct{})

	a.RegisterStep("step1", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release

		return "data", nil
	})

	ctx := t.Context()

	done := make(chan workflow.RunID, 1)

	go func() {
		id, err := a.Start(ctx, "step1", "data", "")
		if err != nil {
			t.Errorf("Start failed: %v", err)
		}

		done <- id
	}()

	<-started

	if err := a.Cancel(ctx, workflow.RunID("run-1")); err != nil {
		t.Fatalf("Cancel while running failed: %v", err)
	}

	var out string

	if err := a.Query(ctx, workflow.RunID("run-1"), "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query cancelled run = %v, want ErrUnknownRun", err)
	}

	if err := a.Signal(ctx, workflow.RunID("run-1"), "advance", nil); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Signal cancelled run = %v, want ErrUnknownRun", err)
	}

	close(release)

	<-done

	_ = a.Close()

	// Cancel after completion must be rejected.
	a2 := newTestAdapter(t)
	a2.RegisterStep("step1", echoStep)

	id2, err := a2.Start(ctx, "step1", "data", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := a2.Cancel(ctx, id2); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Cancel completed run = %v, want ErrRunCompleted", err)
	}
}

func TestSignalUnknownRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	err := a.Signal(ctx, workflow.RunID("nonexistent"), "ev", nil)
	if !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Signal = %v, want ErrUnknownRun", err)
	}

	var unkErr *workflow.UnknownRunError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(%v, *UnknownRunError) = false", err)
	}
}

func TestQueryUnknownRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	var out string

	err := a.Query(ctx, workflow.RunID("nonexistent"), "state", &out)
	if !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query = %v, want ErrUnknownRun", err)
	}
}

func TestCancelUnknownRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	err := a.Cancel(ctx, workflow.RunID("nonexistent"))
	if !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Cancel = %v, want ErrUnknownRun", err)
	}
}

func TestFactoryReturnsAdapter(t *testing.T) {
	t.Parallel()

	f := New

	w, err := f(workflow.Options{})
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}

	if w == nil {
		t.Fatal("expected non-nil workflow")
	}

	if _, ok := w.(*Adapter); !ok {
		t.Fatalf("factory returned %T, want *Adapter", w)
	}
}

func TestSignalDoesNotBlockOnRunningStep(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	a.RegisterStep("blocking", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release

		return "step-result", nil
	})

	var runID workflow.RunID

	startDone := make(chan struct{})

	go func() {
		var err error

		runID, err = a.Start(ctx, "blocking", "input", "")
		if err != nil {
			t.Errorf("Start failed: %v", err)
		}

		close(startDone)
	}()

	<-started

	sigDone := make(chan error, 1)
	go func() {
		sigDone <- a.Signal(ctx, workflow.RunID("run-1"), "advance", "signal-value")
	}()

	select {
	case err := <-sigDone:
		if err != nil {
			t.Fatalf("Signal failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Signal blocked while step running")
	}

	close(release)
	<-startDone

	var out string
	if err := a.Query(ctx, runID, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "signal-value" {
		t.Fatalf("expected signal value to win, got %q", out)
	}

	_ = a.Close()
}

func TestStartUnknownStepErrors(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	runID, err := a.Start(ctx, "nosuchstep", "data", "")
	if !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Start = %v, want ErrUnknownStep", err)
	}

	if runID != "" {
		t.Fatalf("RunID = %q, want empty on failure", runID)
	}

	var out string
	if err := a.Query(ctx, runID, "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query failed run = %v, want ErrUnknownRun", err)
	}
}

func TestStartAfterClose(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id1, err := a.Start(ctx, "step1", "a", "")
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	if cerr := a.Close(); cerr != nil {
		t.Fatalf("Close failed: %v", cerr)
	}

	a.RegisterStep("step1", echoStep)

	id2, err := a.Start(ctx, "step1", "b", "")
	if err != nil {
		t.Fatalf("Start after Close failed: %v", err)
	}

	if id2 == "" || id2 == id1 {
		t.Fatalf("expected fresh RunID after Close, got %q", id2)
	}

	var out string
	if err := a.Query(ctx, id2, "state", &out); err != nil {
		t.Fatalf("Query after Close/Start failed: %v", err)
	}

	if out != "b" {
		t.Fatalf("expected %q, got %q", "b", out)
	}
}

func TestStartAfterCloseWithoutReregister(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	if _, err := a.Start(ctx, "step1", "a", ""); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	if cerr := a.Close(); cerr != nil {
		t.Fatalf("Close failed: %v", cerr)
	}

	// Steps were cleared by Close; Start must re-init maps and report unknown step.
	if _, err := a.Start(ctx, "step1", "b", ""); !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Start = %v, want ErrUnknownStep", err)
	}

	a.RegisterStep("step1", echoStep)

	id, err := a.Start(ctx, "step1", "b", "")
	if err != nil {
		t.Fatalf("Start after re-register failed: %v", err)
	}

	var out string
	if err := a.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "b" {
		t.Fatalf("expected %q, got %q", "b", out)
	}
}

func TestStartStepFailureRemovesRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	boom := errors.New("boom")
	a.RegisterStep("failing", func(_ context.Context, _ any) (any, error) {
		return nil, boom
	})

	ctx := t.Context()

	id, err := a.Start(ctx, "failing", "data", "fail-id")
	if err == nil {
		t.Fatal("expected step failure error, got nil")
	}

	if !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want wrapped %v", err, boom)
	}

	if id != "" {
		t.Fatalf("RunID = %q, want empty on failure", id)
	}

	var out string
	if qerr := a.Query(ctx, "fail-id", "state", &out); !errors.Is(qerr, workflow.ErrUnknownRun) {
		t.Fatalf("Query failed run = %v, want ErrUnknownRun", qerr)
	}

	a.RegisterStep("failing", func(_ context.Context, _ any) (any, error) {
		return "second", nil
	})

	id2, err := a.Start(ctx, "failing", "retry", "fail-id")
	if err != nil {
		t.Fatalf("re-Start with same workflow ID failed: %v", err)
	}

	if id2 != "fail-id" {
		t.Fatalf("unexpected RunID: %q", id2)
	}

	if qerr := a.Query(ctx, "fail-id", "state", &out); qerr != nil {
		t.Fatalf("Query after re-Start failed: %v", qerr)
	}

	if out != "second" {
		t.Fatalf("expected %q, got %q", "second", out)
	}
}

func TestStartAutoSkipsUserSuppliedID(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	userID, err := a.Start(ctx, "step1", "user-run", "run-5")
	if err != nil {
		t.Fatalf("Start with workflow ID failed: %v", err)
	}

	if userID != "run-5" {
		t.Fatalf("unexpected RunID: %q", userID)
	}

	a.mu.Lock()
	a.nextID = 5
	a.mu.Unlock()

	autoID, err := a.Start(ctx, "step1", "auto-run", "")
	if err != nil {
		t.Fatalf("auto Start failed: %v", err)
	}

	if autoID == "run-5" {
		t.Fatal("auto ID collided with user-supplied run-5")
	}

	if autoID != "run-6" {
		t.Fatalf("expected auto ID to skip to run-6, got %q", autoID)
	}

	var out string
	if err := a.Query(ctx, "run-5", "state", &out); err != nil {
		t.Fatalf("user run was overwritten: %v", err)
	}

	if out != "user-run" {
		t.Fatalf("expected user run state %q, got %q", "user-run", out)
	}

	if err := a.Query(ctx, autoID, "state", &out); err != nil {
		t.Fatalf("auto run query failed: %v", err)
	}

	if out != "auto-run" {
		t.Fatalf("expected auto run state %q, got %q", "auto-run", out)
	}
}

func TestQueryTypeMismatch(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("intstep", func(_ context.Context, _ any) (any, error) {
		return 42, nil
	})

	ctx := t.Context()

	runID, err := a.Start(ctx, "intstep", 42, "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var out string
	if err := a.Query(ctx, runID, "state", &out); err == nil {
		t.Fatal("expected type-mismatch error")
	}
}

func TestQueryMarshalError(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("funcstep", func(_ context.Context, _ any) (any, error) {
		return func() {}, nil
	})

	ctx := t.Context()

	runID, err := a.Start(ctx, "funcstep", "input", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var out string
	if err := a.Query(ctx, runID, "state", &out); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestQueryUnknownName(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	runID, err := a.Start(ctx, "step1", "data", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var out any
	if err := a.Query(ctx, runID, "bogus", &out); !errors.Is(err, workflow.ErrUnknownQuery) {
		t.Fatalf("Query = %v, want ErrUnknownQuery", err)
	}
}

func TestStartDuplicateWorkflowIDRejected(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "step1", "first", "dup-id")
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	if id != "dup-id" {
		t.Fatalf("unexpected RunID: %q", id)
	}

	_, err = a.Start(ctx, "step1", "second", "dup-id")
	if !errors.Is(err, workflow.ErrDuplicateRun) {
		t.Fatalf("Start = %v, want ErrDuplicateRun", err)
	}

	if !strings.Contains(err.Error(), "duplicate workflow ID") {
		t.Fatalf("expected duplicate workflow ID error, got %q", err)
	}

	var dupErr *workflow.DuplicateRunError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(%v, *DuplicateRunError) = false", err)
	}

	var out string
	if err := a.Query(ctx, "dup-id", "state", &out); err != nil {
		t.Fatalf("Query of original run failed: %v", err)
	}

	if out != "first" {
		t.Fatalf("expected original run state %q, got %q", "first", out)
	}
}

func TestStartUnknownStepPreservesExistingRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "step1", "original", "keep-id")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if id != "keep-id" {
		t.Fatalf("unexpected RunID: %q", id)
	}

	if _, err := a.Start(ctx, "nosuchstep", "boom", "keep-id"); !errors.Is(err, workflow.ErrDuplicateRun) {
		t.Fatalf("Start = %v, want ErrDuplicateRun", err)
	}

	var out string
	if err := a.Query(ctx, "keep-id", "state", &out); err != nil {
		t.Fatalf("pre-existing run was lost: %v", err)
	}

	if out != "original" {
		t.Fatalf("expected original state %q, got %q", "original", out)
	}
}

func TestStartUnknownStepDeletesOnlyOwnRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	id, err := a.Start(ctx, "nosuchstep", "data", "fresh-id")
	if !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Start = %v, want ErrUnknownStep", err)
	}

	if id != "" {
		t.Fatalf("RunID = %q, want empty on failure", id)
	}

	var out string
	if err := a.Query(ctx, "fresh-id", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query failed run = %v, want ErrUnknownRun", err)
	}
}

func TestSignalDuringStepQueuedAndAppliedInOrder(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})

	a.RegisterStep("blocking", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release

		return "step-result", nil
	})

	startDone := make(chan struct{})
	go func() {
		defer close(startDone)

		if _, err := a.Start(ctx, "blocking", "input", "sig-run"); err != nil {
			t.Errorf("Start failed: %v", err)
		}
	}()

	<-started

	if err := a.Signal(ctx, "sig-run", "advance", "first"); err != nil {
		t.Fatalf("first Signal failed: %v", err)
	}

	if err := a.Signal(ctx, "sig-run", "advance", "second"); err != nil {
		t.Fatalf("second Signal failed: %v", err)
	}

	a.mu.Lock()

	r := a.runs["sig-run"]
	if len(r.pending) != 2 || r.pending[0] != "first" || r.pending[1] != "second" {
		a.mu.Unlock()
		t.Fatalf("unexpected queued signals: %v", r.pending)
	}

	if r.state != "input" {
		a.mu.Unlock()
		t.Fatalf("signal clobbered state mid-step, got %v", r.state)
	}
	a.mu.Unlock()

	close(release)
	<-startDone

	var out string
	if err := a.Query(ctx, "sig-run", "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "second" {
		t.Fatalf("expected %q, got %q", "second", out)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if got := a.runs["sig-run"].pending; len(got) != 0 {
		t.Fatalf("pending signals not drained, got %v", got)
	}
}

func TestCancelRemovesRunAndAllowsRestart(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)

	started := make(chan struct{})
	release := make(chan struct{})

	a.RegisterStep("step1", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release

		return "data", nil
	})

	ctx := t.Context()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if _, err := a.Start(ctx, "step1", "data", "cancel-run"); err != nil {
			t.Errorf("Start failed: %v", err)
		}
	}()

	<-started

	if err := a.Cancel(ctx, "cancel-run"); err != nil {
		t.Fatalf("Cancel while running failed: %v", err)
	}

	var out string
	if err := a.Query(ctx, "cancel-run", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query cancelled run = %v, want ErrUnknownRun", err)
	}

	if err := a.Signal(ctx, "cancel-run", "advance", "nope"); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Signal cancelled run = %v, want ErrUnknownRun", err)
	}

	close(release)

	<-done

	a.RegisterStep("step1", func(_ context.Context, _ any) (any, error) {
		return "restarted", nil
	})

	id2, err := a.Start(ctx, "step1", "retry", "cancel-run")
	if err != nil {
		t.Fatalf("re-Start after Cancel failed: %v", err)
	}

	if id2 != "cancel-run" {
		t.Fatalf("unexpected RunID: %q", id2)
	}

	if err := a.Query(ctx, "cancel-run", "state", &out); err != nil {
		t.Fatalf("Query after re-Start failed: %v", err)
	}

	if out != "restarted" {
		t.Fatalf("expected %q, got %q", "restarted", out)
	}
}

func TestCancelMidStepDoesNotClobberRestartedRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})

	a.RegisterStep("slow", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release

		return "stale", nil
	})

	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)

		if _, err := a.Start(ctx, "slow", "first", "race-run"); err != nil {
			t.Errorf("first Start failed: %v", err)
		}
	}()

	<-started

	if err := a.Cancel(ctx, "race-run"); err != nil {
		t.Fatalf("Cancel while running failed: %v", err)
	}

	// Stale step still holds pointer; its completion must not clobber restarted run.
	a.RegisterStep("fast", func(_ context.Context, _ any) (any, error) {
		return "fresh", nil
	})

	if _, err := a.Start(ctx, "fast", "second", "race-run"); err != nil {
		t.Fatalf("re-Start failed: %v", err)
	}

	close(release)
	<-oldDone

	var out string
	if err := a.Query(ctx, "race-run", "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "fresh" {
		t.Fatalf("expected %q, got %q", "fresh", out)
	}

	if err := a.Query(ctx, "race-run", "state", &out); err != nil {
		t.Fatalf("Query after stale step finished failed: %v", err)
	}

	if out != "fresh" {
		t.Fatalf("stale step result clobbered restarted run, got %q", out)
	}

	// Cancel after completion must be rejected.
	if err := a.Cancel(ctx, "race-run"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Cancel completed run = %v, want ErrRunCompleted", err)
	}
}

func TestSignalBufferedNotLost(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	a.RegisterStep("blocking", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release
		return "step-result", nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := a.Start(ctx, "blocking", "input", "buf-run"); err != nil {
			t.Errorf("Start failed: %v", err)
		}
	}()
	<-started

	// Buffer two signals while step is running; none should be lost.
	if err := a.Signal(ctx, "buf-run", "advance", "first"); err != nil {
		t.Fatalf("first Signal failed: %v", err)
	}
	if err := a.Signal(ctx, "buf-run", "advance", "second"); err != nil {
		t.Fatalf("second Signal failed: %v", err)
	}

	a.mu.Lock()
	r := a.runs["buf-run"]
	if len(r.pending) != 2 || r.pending[0] != "first" || r.pending[1] != "second" {
		a.mu.Unlock()
		t.Fatalf("pending not buffered correctly: %v", r.pending)
	}
	a.mu.Unlock()

	close(release)
	<-done

	var out string
	if err := a.Query(ctx, "buf-run", "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// All buffered signals delivered sequentially; last wins.
	if out != "second" {
		t.Fatalf("expected %q, got %q", "second", out)
	}
	a.mu.Lock()
	if len(a.runs["buf-run"].pending) != 0 {
		a.mu.Unlock()
		t.Fatal("pending not drained")
	}
	a.mu.Unlock()

	// Signal after completion must be rejected and not appended.
	if err := a.Signal(ctx, "buf-run", "advance", "third"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Signal completed run = %v, want ErrRunCompleted", err)
	} else if !strings.Contains(err.Error(), "has completed") {
		t.Fatalf("expected completed error, got %q", err)
	}
}

func TestSignalPendingBounded(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})
	a.RegisterStep("blocking", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release
		return "step-result", nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := a.Start(ctx, "blocking", "input", "bound-run"); err != nil {
			t.Errorf("Start failed: %v", err)
		}
	}()
	<-started

	// Fill pending to cap 100.
	for i := 0; i < 100; i++ {
		if err := a.Signal(ctx, "bound-run", "advance", i); err != nil {
			t.Fatalf("Signal %d failed: %v", i, err)
		}
	}
	// 101st must fail with buffer full.
	if err := a.Signal(ctx, "bound-run", "advance", "overflow"); !errors.Is(err, workflow.ErrPendingFull) {
		t.Fatalf("Signal = %v, want ErrPendingFull", err)
	} else if !strings.Contains(err.Error(), "pending buffer full") {
		t.Fatalf("expected buffer full, got %q", err)
	}

	a.mu.Lock()
	if len(a.runs["bound-run"].pending) != 100 {
		a.mu.Unlock()
		t.Fatalf("expected 100 pending, got %d", len(a.runs["bound-run"].pending))
	}
	a.mu.Unlock()

	close(release)
	<-done

	// After completion, pending drained and signal rejected.
	a.mu.Lock()
	if len(a.runs["bound-run"].pending) != 0 {
		a.mu.Unlock()
		t.Fatal("pending not drained after completion")
	}
	a.mu.Unlock()
	if err := a.Signal(ctx, "bound-run", "advance", "after"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Signal completed run = %v, want ErrRunCompleted", err)
	}
}

func TestRegisterStepOverwrite(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	a.RegisterStep("step", func(_ context.Context, _ any) (any, error) {
		return "first", nil
	})
	a.RegisterStep("step", func(_ context.Context, _ any) (any, error) {
		return "second", nil
	})

	id, err := a.Start(ctx, "step", "input", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var out string
	if err := a.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "second" {
		t.Fatalf("expected overwrite to win, got %q", out)
	}
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)

	if err := a.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestConcurrentMixedOps(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step", echoStep)
	ctx := t.Context()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			id, err := a.Start(ctx, "step", i, "")
			if err != nil {
				t.Errorf("Start %d failed: %v", i, err)
				return
			}

			// Step runs synchronously, so the run has completed:
			// Signal and Cancel must report completion, Query must show state.
			if err := a.Signal(ctx, id, "ev", i); !errors.Is(err, workflow.ErrRunCompleted) {
				t.Errorf("Signal %d = %v, want ErrRunCompleted", i, err)
			}

			var out int
			if err := a.Query(ctx, id, "state", &out); err != nil {
				t.Errorf("Query %d failed: %v", i, err)
			} else if out != i {
				t.Errorf("Query %d = %d, want %d", i, out, i)
			}

			if err := a.Cancel(ctx, id); !errors.Is(err, workflow.ErrRunCompleted) {
				t.Errorf("Cancel %d = %v, want ErrRunCompleted", i, err)
			}
		}(i)
	}
	wg.Wait()
}

// TestRegistryIntegration is serial: it mutates the global adapter registry.
func TestRegistryIntegration(t *testing.T) {
	if err := workflow.Register(workflow.Memory, New); err != nil && !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("Register failed: %v", err)
	}

	w, err := workflow.Open(workflow.Memory, workflow.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if w == nil {
		t.Fatal("expected non-nil workflow")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := workflow.Register(workflow.Memory, New); !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("second Register = %v, want ErrDuplicate", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	bad := workflow.Options{HostPort: "://bad::invalid"}
	if err := bad.Validate(); err == nil {
		t.Skip("core Validate accepts all Options; no invalid input to test")
	}

	if _, err := New(bad); err == nil {
		t.Fatal("expected New with invalid Options to fail")
	}

	// Ensure the adapter is registered, then verify Open propagates the error.
	if err := workflow.Register(workflow.Memory, New); err != nil && !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("Register failed: %v", err)
	}

	if _, err := workflow.Open(workflow.Memory, bad); err == nil {
		t.Fatal("expected Open with invalid Options to fail")
	}
}

// TestAdapterImplementsStepRegistrar pins the StepRegistrar contract: hosts
// register steps through the workflow.StepRegistrar interface, never a
// concrete adapter type.
func TestAdapterImplementsStepRegistrar(t *testing.T) {
	t.Parallel()

	var _ workflow.StepRegistrar = (*Adapter)(nil)
}

// TestRegisterStepThroughInterface registers and runs a step using only the
// workflow.Workflow + workflow.StepRegistrar interfaces, the way host apps
// (e.g. examples/demoapp RegisterDemoWorkflow) do.
func TestRegisterStepThroughInterface(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	w, err := New(workflow.Options{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	reg, ok := w.(workflow.StepRegistrar)
	if !ok {
		t.Fatalf("New returned %T, want workflow.StepRegistrar", w)
	}

	reg.RegisterStep("step1", echoStep)

	runID, err := w.Start(ctx, "step1", "hello", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var result string
	if err := w.Query(ctx, runID, "state", &result); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result != "hello" {
		t.Fatalf("state = %q, want %q", result, "hello")
	}
}
