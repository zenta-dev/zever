// Package workflowtest provides the conformance kit third-party workflow
// engines run to prove backend parity.
package workflowtest

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// adapterSeq keeps throwaway registry names unique across Conformance runs
// in one process.
var adapterSeq atomic.Uint64

// Conformance verifies factory-built engines implement the
// workflow.Workflow contract: Start/Query round-trip, duplicate workflow
// IDs, unknown steps/runs/queries, completed-run guards, Open/Register
// registry wiring, and Close. Each subtest takes a fresh instance from
// factory so cases stay isolated. Engines execute the "greet" echo step
// synchronously, so Signal and Cancel against a started run must fail
// closed with ErrRunCompleted; the kit requires factory products to
// implement StepRegistrar and fails otherwise. Tests never touch the
// network and never synchronize with time.Sleep.
func Conformance(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	t.Run("StartQuery", func(t *testing.T) { conformanceStartQuery(t, factory) })
	t.Run("DuplicateRun", func(t *testing.T) { conformanceDuplicateRun(t, factory) })
	t.Run("UnknownStep", func(t *testing.T) { conformanceUnknownStep(t, factory) })
	t.Run("UnknownRun", func(t *testing.T) { conformanceUnknownRun(t, factory) })
	t.Run("CompletedGuards", func(t *testing.T) { conformanceCompletedGuards(t, factory) })
	t.Run("UnknownQuery", func(t *testing.T) { conformanceUnknownQuery(t, factory) })
	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

// greet echoes its input. It is JSON-marshalable both ways so memory and
// DB-backed engines agree.
func greet(_ context.Context, input any) (any, error) { return input, nil }

// withGreet registers the echo step, failing when the engine does not
// implement StepRegistrar.
func withGreet(t *testing.T, w workflow.Workflow) {
	t.Helper()

	reg, ok := w.(workflow.StepRegistrar)
	if !ok {
		t.Fatalf("%T does not implement StepRegistrar", w)
	}

	reg.RegisterStep("greet", greet)
}

func conformanceStartQuery(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	id, err := w.Start(ctx, "greet", "hello", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if id == "" {
		t.Fatal("Start() returned empty RunID")
	}

	var out string
	if qerr := w.Query(ctx, id, "state", &out); qerr != nil {
		t.Fatalf("Query() error = %v", qerr)
	}

	if out != "hello" {
		t.Errorf("Query() = %q, want hello", out)
	}

	named, err := w.Start(ctx, "greet", "named", "kit-run-1")
	if err != nil {
		t.Fatalf("Start(named) error = %v", err)
	}

	if named != "kit-run-1" {
		t.Errorf("Start(named) = %q, want kit-run-1", named)
	}
}

func conformanceDuplicateRun(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	if _, err := w.Start(ctx, "greet", "first", "kit-dupe"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, err := w.Start(ctx, "greet", "second", "kit-dupe"); !errors.Is(err, workflow.ErrDuplicateRun) {
		t.Fatalf("Start(dup) err = %v, want ErrDuplicateRun", err)
	}

	var out string
	if err := w.Query(ctx, "kit-dupe", "state", &out); err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if out != "first" {
		t.Errorf("Query() = %q, want first (original preserved)", out)
	}
}

func conformanceUnknownStep(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	if _, err := w.Start(ctx, "kit-no-such-step", "x", ""); !errors.Is(err, workflow.ErrUnknownStep) {
		t.Errorf("Start(unknown step) err = %v, want ErrUnknownStep", err)
	}
}

func conformanceUnknownRun(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	missing := workflow.RunID("kit-no-such-run")

	if err := w.Signal(ctx, missing, "advance", "x"); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Errorf("Signal(missing) err = %v, want ErrUnknownRun", err)
	}

	var out string
	if err := w.Query(ctx, missing, "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Errorf("Query(missing) err = %v, want ErrUnknownRun", err)
	}

	if err := w.Cancel(ctx, missing); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Errorf("Cancel(missing) err = %v, want ErrUnknownRun", err)
	}
}

func conformanceCompletedGuards(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	id, err := w.Start(ctx, "greet", "hi", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Engines run the step synchronously, so the run is completed: Signal
	// and Cancel must fail closed instead of reporting silent success.
	if err := w.Signal(ctx, id, "advance", "late"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Errorf("Signal(completed) err = %v, want ErrRunCompleted", err)
	}

	if err := w.Cancel(ctx, id); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Errorf("Cancel(completed) err = %v, want ErrRunCompleted", err)
	}
}

func conformanceUnknownQuery(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	ctx := t.Context()
	w := factory(t)
	withGreet(t, w)

	id, err := w.Start(ctx, "greet", "hi", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	var out string
	if err := w.Query(ctx, id, "kit-no-such-query", &out); !errors.Is(err, workflow.ErrUnknownQuery) {
		t.Errorf("Query(unknown) err = %v, want ErrUnknownQuery", err)
	}
}

func conformanceOpenRegister(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	name := workflow.Adapter("kit-open-test-" + strconv.FormatUint(adapterSeq.Add(1), 10))
	probe := factory(t)

	if err := workflow.Register(name, func(workflow.Options) (workflow.Workflow, error) { return probe, nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := workflow.Register(name, func(workflow.Options) (workflow.Workflow, error) { return probe, nil }); !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("Register(dup) err = %v, want ErrDuplicate", err)
	}

	opened, err := workflow.Open(name, workflow.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if opened != probe {
		t.Error("Open() did not return the registered engine")
	}

	if _, err := workflow.Open("kit-no-such-adapter", workflow.Options{}); !errors.Is(err, workflow.ErrUnknownAdapter) {
		t.Errorf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) workflow.Workflow) {
	t.Helper()

	w := factory(t)

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
