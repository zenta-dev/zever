package memory

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func newSagaAdapter(t *testing.T) *Adapter {
	t.Helper()

	w, err := New(workflow.Options{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	a, ok := w.(*Adapter)
	if !ok {
		t.Fatalf("New returned %T, want *Adapter", w)
	}

	t.Cleanup(func() { _ = a.Close() })

	return a
}

func mustRegisterSaga(t *testing.T, a *Adapter, name string, steps ...workflow.SagaStep) {
	t.Helper()

	a.RegisterSaga(name, steps)
}

func mustSagaStatus(t *testing.T, a *Adapter, id workflow.RunID) workflow.SagaStatus {
	t.Helper()

	st, err := a.SagaStatus(t.Context(), id)
	if err != nil {
		t.Fatalf("SagaStatus(%q) failed: %v", id, err)
	}

	return st
}

func TestSagaHappyPath(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, a, "ok",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "a")

			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "b")

			return "rb", nil
		}},
	)

	id, err := a.RunSaga(ctx, "ok", "input", "wf-ok")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaCompleted {
		t.Fatalf("status = %q, want %q", st.Status, workflow.SagaCompleted)
	}
	if st.CurrentStep != 2 {
		t.Errorf("currentStep = %d, want 2", st.CurrentStep)
	}
	if st.FailedStep != -1 {
		t.Errorf("failedStep = %d, want -1", st.FailedStep)
	}
	if st.Name != "ok" {
		t.Errorf("name = %q, want %q", st.Name, "ok")
	}
	if strings.Join(order, ",") != "a,b" {
		t.Errorf("execute order = %v, want [a b]", order)
	}
}

func TestSagaFailureCompensatesInReverse(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, a, "comp",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-a")

			return "ra", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			order = append(order, "comp-a")

			return struct{}{}, nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-b")

			return "rb", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			order = append(order, "comp-b")

			return struct{}{}, nil
		}},
		workflow.SagaStep{Name: "c", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-c")

			return nil, errors.New("boom")
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			order = append(order, "comp-c")

			return struct{}{}, nil
		}},
	)

	id, err := a.RunSaga(ctx, "comp", "input", "wf-comp")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaFailed {
		t.Fatalf("status = %q, want %q", st.Status, workflow.SagaFailed)
	}
	if st.FailedStep != 2 {
		t.Errorf("failedStep = %d, want 2", st.FailedStep)
	}
	if !strings.Contains(st.Err, "boom") {
		t.Errorf("err = %q, want to contain %q", st.Err, "boom")
	}

	want := "exec-a,exec-b,exec-c,comp-b,comp-a"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("order = %q, want %q (reverse compensation, no comp-c)", got, want)
	}
}

func TestSagaPivotPreventsCompensation(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, a, "pivot",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-a")

			return "ra", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			order = append(order, "comp-a")

			return struct{}{}, nil
		}},
		workflow.SagaStep{Name: "p", Pivot: true, Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-p")

			return "rp", nil
		}},
		workflow.SagaStep{Name: "c", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "exec-c")

			return nil, errors.New("boom")
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			order = append(order, "comp-c")

			return struct{}{}, nil
		}},
	)

	id, err := a.RunSaga(ctx, "pivot", "input", "wf-pivot")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaFailed {
		t.Fatalf("status = %q, want %q (pivot rolls forward, no compensation)", st.Status, workflow.SagaFailed)
	}

	for _, e := range order {
		if strings.HasPrefix(e, "comp-") {
			t.Errorf("compensation ran despite pivot: %v", order)
		}
	}

	if got := strings.Join(order, ","); got != "exec-a,exec-p,exec-c,exec-c,exec-c,exec-c" {
		t.Errorf("order = %q, want exec-a,exec-p plus 3 roll-forward retries of exec-c", order)
	}
}

func TestSagaPivotRollForwardRetries(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	var attempts atomic.Int64

	mustRegisterSaga(t, a, "pivot-retry",
		workflow.SagaStep{Name: "p", Pivot: true, Execute: func(_ context.Context, _ any) (any, error) {
			return "rp", nil
		}},
		workflow.SagaStep{Name: "flaky", Execute: func(_ context.Context, _ any) (any, error) {
			if attempts.Add(1) < 3 {
				return nil, errors.New("transient")
			}

			return "ok", nil
		}},
	)

	id, err := a.RunSaga(ctx, "pivot-retry", "input", "wf-pivot-retry")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaCompleted {
		t.Fatalf("status = %q, want %q (roll-forward retry succeeds)", st.Status, workflow.SagaCompleted)
	}
	if n := attempts.Load(); n != 3 {
		t.Errorf("flaky attempts = %d, want 3", n)
	}
}

func TestSagaCompensationFailure(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	mustRegisterSaga(t, a, "comp-fail",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			return "ra", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("comp boom")
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("exec boom")
		}},
	)

	id, err := a.RunSaga(ctx, "comp-fail", "input", "wf-comp-fail")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaCompensationFailed {
		t.Fatalf("status = %q, want %q", st.Status, workflow.SagaCompensationFailed)
	}
	if !strings.Contains(st.Err, "comp boom") {
		t.Errorf("err = %q, want to contain %q", st.Err, "comp boom")
	}
}

func TestSagaUnknownSaga(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)

	if _, err := a.RunSaga(t.Context(), "nosuch", "input", ""); !errors.Is(err, workflow.ErrUnknownSaga) {
		t.Fatalf("RunSaga = %v, want ErrUnknownSaga", err)
	}
}

func TestSagaResumeSameWorkflowID(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	var calls atomic.Int64

	mustRegisterSaga(t, a, "resume",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return nil, errors.New("boom")
		}},
	)

	id1, err := a.RunSaga(ctx, "resume", "input", "wf-resume")
	if err != nil {
		t.Fatalf("first RunSaga failed: %v", err)
	}

	id2, err := a.RunSaga(ctx, "resume", "input", "wf-resume")
	if err != nil {
		t.Fatalf("resume RunSaga failed: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("resume runID = %q, want same %q", id2, id1)
	}

	if n := calls.Load(); n != 2 {
		t.Errorf("steps executed %d times total, want 2 (no re-run on resume)", n)
	}

	st := mustSagaStatus(t, a, id1)
	if st.Status != workflow.SagaFailed {
		t.Errorf("status = %q, want %q", st.Status, workflow.SagaFailed)
	}
}

func TestSagaStatusUnknownRun(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)

	if _, err := a.SagaStatus(t.Context(), "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("SagaStatus = %v, want ErrSagaNotFound", err)
	}
}

func TestSagaNilCompensateSkipped(t *testing.T) {
	t.Parallel()

	a := newSagaAdapter(t)
	ctx := t.Context()

	mustRegisterSaga(t, a, "nil-comp",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("boom")
		}},
	)

	id, err := a.RunSaga(ctx, "nil-comp", "input", "wf-nil-comp")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, a, id)
	if st.Status != workflow.SagaFailed {
		t.Fatalf("status = %q, want %q (nil Compensate skipped)", st.Status, workflow.SagaFailed)
	}
}
