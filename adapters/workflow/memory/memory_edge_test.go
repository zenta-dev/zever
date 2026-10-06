package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// mustRunningRun starts a run whose step blocks until the returned release
// function is called, leaving the run in the running state for edge
// assertions. The release function also joins the Start goroutine.
func mustRunningRun(t *testing.T, a *Adapter, step, id string) func() {
	t.Helper()

	started := make(chan struct{})
	releaseCh := make(chan struct{})

	a.RegisterStep(step, func(_ context.Context, _ any) (any, error) {
		close(started)
		<-releaseCh

		return "done", nil
	})

	ctx := t.Context()
	done := make(chan error, 1)

	go func() {
		_, err := a.Start(ctx, step, "input", id)
		done <- err
	}()

	<-started

	return func() {
		close(releaseCh)

		if err := <-done; err != nil {
			t.Errorf("Start(%s) error = %v", step, err)
		}
	}
}

// TestRegister_opensViaCoreOptions proves Register wires the memory engine
// into the core registry so workflow.Open resolves it.
func TestRegister_opensViaCoreOptions(t *testing.T) {
	Register()

	w, err := workflow.Open(workflow.Memory, workflow.Options{})
	if err != nil {
		t.Fatalf("workflow.Open(memory) = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })
}

// TestEdgeStartNilInput stores a nil input and decodes it back as nil.
func TestEdgeStartNilInput(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("nilstep", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "nilstep", nil, "")
	if err != nil {
		t.Fatalf("Start(nil) error = %v", err)
	}

	var out any
	if err := a.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query error = %v", err)
	}

	if out != nil {
		t.Fatalf("state = %v, want nil", out)
	}

	_ = a.Close()
}

// TestEdgeQueryNonPointerTarget rejects a non-pointer decode target.
func TestEdgeQueryNonPointerTarget(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "step1", "value", "")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	var out string
	if err := a.Query(ctx, id, "state", out); err == nil {
		t.Fatal("Query(non-pointer) = nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("Query(non-pointer) = %q, want type mismatch", err)
	}

	_ = a.Close()
}

// TestEdgeQueryNilTarget rejects a nil decode target.
func TestEdgeQueryNilTarget(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "step1", "value", "")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if err := a.Query(ctx, id, "state", nil); err == nil {
		t.Fatal("Query(nil) = nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("Query(nil) = %q, want type mismatch", err)
	}

	_ = a.Close()
}

// TestEdgeQueryEmptyName rejects an empty query name with the typed error.
func TestEdgeQueryEmptyName(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("step1", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "step1", "value", "")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	var out string
	if err := a.Query(ctx, id, "", &out); !errors.Is(err, workflow.ErrUnknownQuery) {
		t.Fatalf("Query(empty) = %v, want ErrUnknownQuery", err)
	} else {
		var qErr workflow.UnknownQueryError
		if !errors.As(err, &qErr) || qErr.Query != "" {
			t.Fatalf("errors.As(%v, *UnknownQueryError) = %v, want empty Query", err, qErr)
		}
	}

	_ = a.Close()
}

// TestEdgeStartEmptyStepNameUnregistered reports the empty name as unknown.
func TestEdgeStartEmptyStepNameUnregistered(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	if _, err := a.Start(ctx, "", "value", ""); !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Start(empty) = %v, want ErrUnknownStep", err)
	} else {
		var sErr workflow.UnknownStepError
		if !errors.As(err, &sErr) || sErr.Step != "" {
			t.Fatalf("errors.As(%v, *UnknownStepError) = %v, want empty Step", err, sErr)
		}
	}

	_ = a.Close()
}

// TestEdgeRegisterEmptyStepName allows the empty name to be registered.
func TestEdgeRegisterEmptyStepName(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	a.RegisterStep("", echoStep)
	ctx := t.Context()

	id, err := a.Start(ctx, "", "value", "")
	if err != nil {
		t.Fatalf("Start(empty) error = %v", err)
	}

	var out string
	if err := a.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query error = %v", err)
	}

	if out != "value" {
		t.Fatalf("state = %q, want %q", out, "value")
	}

	_ = a.Close()
}

// TestEdgeSignalNilValueWithEmptyName buffers a nil value under an empty
// channel name; both are ignored/accepted by the engine.
func TestEdgeSignalNilValueWithEmptyName(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	release := mustRunningRun(t, a, "block", "nil-sig")
	ctx := t.Context()

	if err := a.Signal(ctx, "nil-sig", "", nil); err != nil {
		t.Fatalf("Signal(nil) error = %v", err)
	}

	a.mu.RLock()
	pending := a.runs["nil-sig"].pending
	a.mu.RUnlock()

	if len(pending) != 1 || pending[0] != nil {
		t.Fatalf("pending = %v, want [<nil>]", pending)
	}

	release()

	var out any
	if err := a.Query(ctx, "nil-sig", "state", &out); err != nil {
		t.Fatalf("Query error = %v", err)
	}

	if out != nil {
		t.Fatalf("final state = %v, want nil", out)
	}

	_ = a.Close()
}

// TestEdgeOpsAfterCloseUnknownRun fails closed once Close has cleared the maps.
func TestEdgeOpsAfterCloseUnknownRun(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	ctx := t.Context()

	if err := a.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	if err := a.Signal(ctx, "gone", "ev", nil); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Signal after Close = %v, want ErrUnknownRun", err)
	}

	var out string
	if err := a.Query(ctx, "gone", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query after Close = %v, want ErrUnknownRun", err)
	}

	if err := a.Cancel(ctx, "gone"); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Cancel after Close = %v, want ErrUnknownRun", err)
	}
}

// TestEdgeConcurrentSignalBoundary documents goroutine-safety: concurrent
// Signals on one running run deliver exactly the pending cap of 100 values;
// every excess caller fails with ErrPendingFull.
func TestEdgeConcurrentSignalBoundary(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t)
	release := mustRunningRun(t, a, "block", "bound-run")
	ctx := t.Context()

	const callers = 200

	var (
		wg   sync.WaitGroup
		ok   atomic.Int64
		full atomic.Int64
	)

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			switch err := a.Signal(ctx, "bound-run", "advance", i); {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, workflow.ErrPendingFull):
				full.Add(1)
			default:
				t.Errorf("Signal(%d) = %v, want nil or ErrPendingFull", i, err)
			}
		}(i)
	}

	wg.Wait()

	if got := ok.Load(); got != 100 {
		t.Fatalf("successful Signals = %d, want 100", got)
	}

	if got := full.Load(); got != callers-100 {
		t.Fatalf("ErrPendingFull Signals = %d, want %d", got, callers-100)
	}

	release()
	_ = a.Close()
}
