package workflow_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/workflow"
)

var workflowAdapterSeq atomic.Int64

func freshWorkflowAdapter() workflow.Adapter {
	return workflow.Adapter(1000 + workflowAdapterSeq.Add(1))
}

type stubWorkflow struct{}

func (stubWorkflow) Start(_ context.Context, _ string, _ any, _ string) (workflow.RunID, error) {
	return "run-1", nil
}

func (stubWorkflow) Signal(_ context.Context, _ workflow.RunID, _ string, _ any) error { return nil }

func (stubWorkflow) Query(_ context.Context, _ workflow.RunID, _ string, _ any) error { return nil }

func (stubWorkflow) Cancel(_ context.Context, _ workflow.RunID) error { return nil }

func (stubWorkflow) Close() error { return nil }

func TestWorkflowRegisterNil(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()

	err := workflow.Register(a, nil)
	if !errors.Is(err, workflow.ErrNilFactory) {
		t.Fatalf("Register(nil) = %v, want ErrNilFactory", err)
	}
}

func TestWorkflowRegisterDuplicate(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	stub := func(workflow.Options) (workflow.Workflow, error) { return stubWorkflow{}, nil }

	if err := workflow.Register(a, stub); err != nil {
		t.Fatalf("first Register(%v) = %v, want nil", a, err)
	}

	err := workflow.Register(a, stub)
	if !errors.Is(err, workflow.ErrDuplicate) {
		t.Fatalf("second Register(%v) = %v, want ErrDuplicate", a, err)
	}

	var dupErr *workflow.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(%v, *DuplicateError) = false", err)
	}

	if dupErr.Adapter != a {
		t.Fatalf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

func TestWorkflowOpenUnknown(t *testing.T) {
	t.Parallel()

	w, err := workflow.Open(workflow.Adapter(9999), workflow.Options{})
	if w != nil {
		t.Fatalf("Open(unknown) = %v, want nil", w)
	}

	if !errors.Is(err, workflow.ErrUnknownAdapter) {
		t.Fatalf("Open(unknown) = %v, want ErrUnknownAdapter", err)
	}

	var unkErr *workflow.UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(%v, *UnknownAdapterError) = false", err)
	}
}

func TestWorkflowOpenFactoryError(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	sentinel := errors.New("boom")

	if err := workflow.Register(a, func(workflow.Options) (workflow.Workflow, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("Register(%v) = %v, want nil", a, err)
	}

	w, err := workflow.Open(a, workflow.Options{})
	if w != nil {
		t.Fatalf("Open factory error = %v, want nil value", w)
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("Open factory error = %v, want sentinel %v", err, sentinel)
	}

	if !strings.Contains(err.Error(), "workflow: open") {
		t.Fatalf("Open factory error %q does not contain %q", err.Error(), "workflow: open")
	}
}

func TestWorkflowOpenSuccess(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	wantOpts := workflow.Options{HostPort: "localhost:7233", Namespace: "default"}
	var gotOpts workflow.Options

	factory := func(opts workflow.Options) (workflow.Workflow, error) {
		gotOpts = opts
		return stubWorkflow{}, nil
	}

	if err := workflow.Register(a, factory); err != nil {
		t.Fatalf("Register(%v) = %v, want nil", a, err)
	}

	w, err := workflow.Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open(%v) = %v, want nil", a, err)
	}

	if w == nil {
		t.Fatalf("Open(%v) returned nil Workflow", a)
	}

	if gotOpts != wantOpts {
		t.Fatalf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestWorkflowOpenValidateFailure(t *testing.T) {
	t.Parallel()

	a := freshWorkflowAdapter()
	called := false

	if err := workflow.Register(a, func(workflow.Options) (workflow.Workflow, error) {
		called = true
		return stubWorkflow{}, nil
	}); err != nil {
		t.Fatalf("Register(%v) = %v, want nil", a, err)
	}

	w, err := workflow.Open(a, workflow.Options{HostPort: "missing-port"})
	if w != nil {
		t.Fatalf("Open invalid opts = %v, want nil", w)
	}

	if !errors.Is(err, workflow.ErrInvalidOptions) {
		t.Fatalf("Open invalid opts = %v, want ErrInvalidOptions", err)
	}

	if called {
		t.Fatalf("factory called despite Validate failure")
	}
}
