package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
)

func mustRegisterSaga(t *testing.T, d *driver, name string, steps ...workflow.SagaStep) {
	t.Helper()

	d.RegisterSaga(name, steps)
}

func mustSagaStatus(t *testing.T, d *driver, id workflow.RunID) workflow.SagaStatus {
	t.Helper()

	st, err := d.SagaStatus(t.Context(), id)
	if err != nil {
		t.Fatalf("SagaStatus(%q) failed: %v", id, err)
	}

	return st
}

func TestSagaHappyPath(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, d, "ok",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "a")

			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			order = append(order, "b")

			return "rb", nil
		}},
	)

	id, err := d.RunSaga(ctx, "ok", "input", "wf-ok")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, d, id)
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

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, d, "comp",
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

	id, err := d.RunSaga(ctx, "comp", "input", "wf-comp")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, d, id)
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

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var order []string

	mustRegisterSaga(t, d, "pivot",
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

	id, err := d.RunSaga(ctx, "pivot", "input", "wf-pivot")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, d, id)
	if st.Status != workflow.SagaFailed {
		t.Fatalf("status = %q, want %q (pivot rolls forward, no compensation)", st.Status, workflow.SagaFailed)
	}

	for _, e := range order {
		if strings.HasPrefix(e, "comp-") {
			t.Errorf("compensation ran despite pivot: %v", order)
		}
	}
}

func TestSagaPivotRollForwardRetries(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var attempts atomic.Int64

	mustRegisterSaga(t, d, "pivot-retry",
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

	id, err := d.RunSaga(ctx, "pivot-retry", "input", "wf-pivot-retry")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, d, id)
	if st.Status != workflow.SagaCompleted {
		t.Fatalf("status = %q, want %q (roll-forward retry succeeds)", st.Status, workflow.SagaCompleted)
	}
	if n := attempts.Load(); n != 3 {
		t.Errorf("flaky attempts = %d, want 3", n)
	}
}

func TestSagaCompensationFailure(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	mustRegisterSaga(t, d, "comp-fail",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			return "ra", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("comp boom")
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("exec boom")
		}},
	)

	id, err := d.RunSaga(ctx, "comp-fail", "input", "wf-comp-fail")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st := mustSagaStatus(t, d, id)
	if st.Status != workflow.SagaCompensationFailed {
		t.Fatalf("status = %q, want %q", st.Status, workflow.SagaCompensationFailed)
	}

	rows, err := orm.From[sagaCompensationRow, *sagaCompensationRow](d.sagaCompTbl).
		Where(d.sagaCompRunID.Eq(string(id))).All(ctx, d.conn)
	if err != nil {
		t.Fatalf("load compensations failed: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("compensation rows = %d, want 1", len(rows))
	}

	if rows[0].StepIndex != 0 || rows[0].Name != "a" {
		t.Errorf("compensation row = %+v, want step 0 a", rows[0])
	}

	if !strings.Contains(rows[0].LastError, "comp boom") {
		t.Errorf("last_error = %q, want to contain %q", rows[0].LastError, "comp boom")
	}

	if rows[0].Attempts < 2 {
		t.Errorf("attempts = %d, want >= 2 (retried)", rows[0].Attempts)
	}
}

func TestSagaUnknownSaga(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})

	if _, err := d.RunSaga(t.Context(), "nosuch", "input", ""); !errors.Is(err, workflow.ErrUnknownSaga) {
		t.Fatalf("RunSaga = %v, want ErrUnknownSaga", err)
	}
}

func TestSagaResumeSameWorkflowID(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var calls atomic.Int64

	mustRegisterSaga(t, d, "resume",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return nil, errors.New("boom")
		}},
	)

	id1, err := d.RunSaga(ctx, "resume", "input", "wf-resume")
	if err != nil {
		t.Fatalf("first RunSaga failed: %v", err)
	}

	id2, err := d.RunSaga(ctx, "resume", "input", "wf-resume")
	if err != nil {
		t.Fatalf("resume RunSaga failed: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("resume runID = %q, want same %q", id2, id1)
	}

	if n := calls.Load(); n != 2 {
		t.Errorf("steps executed %d times total, want 2 (no re-run on resume)", n)
	}

	st := mustSagaStatus(t, d, id1)
	if st.Status != workflow.SagaFailed {
		t.Errorf("status = %q, want %q", st.Status, workflow.SagaFailed)
	}
}

func TestSagaStatusUnknownRun(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})

	if _, err := d.SagaStatus(t.Context(), "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("SagaStatus = %v, want ErrSagaNotFound", err)
	}
}

func TestRecoverStuckSagas(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var calls atomic.Int64

	mustRegisterSaga(t, d, "recover",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return "ra", nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			calls.Add(1)

			return "rb", nil
		}},
	)

	// Simulate a crashed run: step a done, step b never ran, lease expired.
	now := time.Now().UTC()

	err := orm.InsertInto(d.sagaTbl).Values(
		orm.Set(d.sagaRunID, "saga-stuck"),
		orm.Set(d.sagaName, "recover"),
		orm.Set(d.sagaWorkflowID, "wf-stuck"),
		orm.Set(d.sagaStatus, sagaStateRunning),
		orm.Set(d.sagaCurrentStep, 1),
		orm.Set(d.sagaStateCol, `{"input":"go","results":["ra"]}`),
		orm.Set(d.sagaFailedStep, -1),
		orm.Set(d.sagaLockedUntil, now.Add(-time.Hour)),
		orm.Set(d.sagaCreatedAt, now.Add(-time.Hour)),
		orm.Set(d.sagaUpdatedAt, now.Add(-time.Hour)),
	).Exec(ctx, d.conn)
	if err != nil {
		t.Fatalf("insert stuck saga failed: %v", err)
	}

	// A live run (unexpired lease) must not be recovered.
	err = orm.InsertInto(d.sagaTbl).Values(
		orm.Set(d.sagaRunID, "saga-live"),
		orm.Set(d.sagaName, "recover"),
		orm.Set(d.sagaWorkflowID, "wf-live"),
		orm.Set(d.sagaStatus, sagaStateRunning),
		orm.Set(d.sagaCurrentStep, 1),
		orm.Set(d.sagaStateCol, `{"input":"go","results":["ra"]}`),
		orm.Set(d.sagaFailedStep, -1),
		orm.Set(d.sagaLockedUntil, now.Add(time.Hour)),
		orm.Set(d.sagaCreatedAt, now),
		orm.Set(d.sagaUpdatedAt, now),
	).Exec(ctx, d.conn)
	if err != nil {
		t.Fatalf("insert live saga failed: %v", err)
	}

	n, err := d.RecoverStuckSagas(ctx, 0)
	if err != nil {
		t.Fatalf("RecoverStuckSagas failed: %v", err)
	}

	if n != 1 {
		t.Fatalf("recovered = %d, want 1 (only the expired run)", n)
	}

	st := mustSagaStatus(t, d, "saga-stuck")
	if st.Status != workflow.SagaCompleted {
		t.Fatalf("stuck saga status = %q, want %q", st.Status, workflow.SagaCompleted)
	}

	if st.CurrentStep != 2 {
		t.Errorf("stuck saga currentStep = %d, want 2", st.CurrentStep)
	}

	if nc := calls.Load(); nc != 1 {
		t.Errorf("step b executed %d times, want 1 (step a not re-run)", nc)
	}

	live := mustSagaStatus(t, d, "saga-live")
	if live.Status != workflow.SagaRunning {
		t.Errorf("live saga status = %q, want %q (untouched)", live.Status, workflow.SagaRunning)
	}
}

func TestRecoverStuckSagasCompensating(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga"})
	ctx := t.Context()

	var compCalls atomic.Int64

	mustRegisterSaga(t, d, "recover-comp",
		workflow.SagaStep{Name: "a", Execute: func(_ context.Context, _ any) (any, error) {
			return "ra", nil
		}, Compensate: func(_ context.Context, _ any) (any, error) {
			compCalls.Add(1)

			return struct{}{}, nil
		}},
		workflow.SagaStep{Name: "b", Execute: func(_ context.Context, _ any) (any, error) {
			return nil, errors.New("boom")
		}},
	)

	// Crashed mid-compensation: step a executed, failure at step b recorded,
	// compensation of step a never ran.
	now := time.Now().UTC()

	err := orm.InsertInto(d.sagaTbl).Values(
		orm.Set(d.sagaRunID, "saga-stuck-comp"),
		orm.Set(d.sagaName, "recover-comp"),
		orm.Set(d.sagaWorkflowID, "wf-stuck-comp"),
		orm.Set(d.sagaStatus, sagaStateCompensating),
		orm.Set(d.sagaCurrentStep, 1),
		orm.Set(d.sagaStateCol, `{"input":"go","results":["ra"]}`),
		orm.Set(d.sagaFailedStep, 1),
		orm.Set(d.sagaLockedUntil, now.Add(-time.Hour)),
		orm.Set(d.sagaCreatedAt, now.Add(-time.Hour)),
		orm.Set(d.sagaUpdatedAt, now.Add(-time.Hour)),
	).Exec(ctx, d.conn)
	if err != nil {
		t.Fatalf("insert stuck saga failed: %v", err)
	}

	n, err := d.RecoverStuckSagas(ctx, 0)
	if err != nil {
		t.Fatalf("RecoverStuckSagas failed: %v", err)
	}

	if n != 1 {
		t.Fatalf("recovered = %d, want 1", n)
	}

	st := mustSagaStatus(t, d, "saga-stuck-comp")
	if st.Status != workflow.SagaFailed {
		t.Fatalf("status = %q, want %q (compensation finished)", st.Status, workflow.SagaFailed)
	}

	if c := compCalls.Load(); c != 1 {
		t.Errorf("compensate executed %d times, want 1", c)
	}
}

func TestSagaImplementsContracts(t *testing.T) {
	t.Parallel()

	var _ workflow.SagaRegistrar = (*driver)(nil)
	var _ workflow.SagaRunner = (*driver)(nil)
	var _ workflow.SagaInspector = (*driver)(nil)
}

func TestSagaRegisterViaWorkflowOpen(t *testing.T) {
	t.Parallel()

	Register()

	wf, err := workflow.Open(workflow.DB, workflow.Options{})
	if err != nil {
		t.Fatalf("Open(db) failed: %v", err)
	}

	t.Cleanup(func() { _ = wf.Close() })

	reg, ok := wf.(workflow.SagaRegistrar)
	if !ok {
		t.Fatalf("db workflow %T does not implement SagaRegistrar", wf)
	}

	runner, ok := wf.(workflow.SagaRunner)
	if !ok {
		t.Fatalf("db workflow %T does not implement SagaRunner", wf)
	}

	inspector, ok := wf.(workflow.SagaInspector)
	if !ok {
		t.Fatalf("db workflow %T does not implement SagaInspector", wf)
	}

	reg.RegisterSaga("open-saga", []workflow.SagaStep{
		{Name: "a", Execute: func(_ context.Context, input any) (any, error) {
			return fmt.Sprintf("done-%v", input), nil
		}},
	})

	id, err := runner.RunSaga(t.Context(), "open-saga", "x", "wf-open")
	if err != nil {
		t.Fatalf("RunSaga failed: %v", err)
	}

	st, err := inspector.SagaStatus(t.Context(), id)
	if err != nil {
		t.Fatalf("SagaStatus failed: %v", err)
	}

	if st.Status != workflow.SagaCompleted {
		t.Errorf("status = %q, want %q", st.Status, workflow.SagaCompleted)
	}
}
