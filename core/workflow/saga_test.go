package workflow_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func TestSagaStatusConsts(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"SagaPending":            workflow.SagaPending,
		"SagaRunning":            workflow.SagaRunning,
		"SagaCompleted":          workflow.SagaCompleted,
		"SagaCompensating":       workflow.SagaCompensating,
		"SagaFailed":             workflow.SagaFailed,
		"SagaCompensationFailed": workflow.SagaCompensationFailed,
	}

	want := map[string]string{
		"SagaPending":            "pending",
		"SagaRunning":            "running",
		"SagaCompleted":          "completed",
		"SagaCompensating":       "compensating",
		"SagaFailed":             "failed",
		"SagaCompensationFailed": "compensation_failed",
	}

	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestSagaStepFields(t *testing.T) {
	t.Parallel()

	exec := workflow.StepFunc(func(_ context.Context, input any) (any, error) {
		return input, nil
	})
	comp := workflow.StepFunc(func(_ context.Context, _ any) (any, error) {
		return struct{}{}, nil
	})

	step := workflow.SagaStep{Name: "charge", Execute: exec, Compensate: comp, Pivot: true}

	if step.Name != "charge" {
		t.Errorf("Name = %q, want %q", step.Name, "charge")
	}
	if step.Execute == nil {
		t.Error("Execute is nil")
	}
	if step.Compensate == nil {
		t.Error("Compensate is nil")
	}
	if !step.Pivot {
		t.Error("Pivot = false, want true")
	}

	out, err := step.Execute(t.Context(), "x")
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if out != "x" {
		t.Errorf("Execute result = %v, want %q", out, "x")
	}
}

func TestSagaStatusZero(t *testing.T) {
	t.Parallel()

	var st workflow.SagaStatus

	if st.FailedStep != 0 {
		t.Errorf("zero FailedStep = %d, want 0", st.FailedStep)
	}
	if st.Status != "" {
		t.Errorf("zero Status = %q, want empty", st.Status)
	}
}

func TestSagaInterfacesCompile(t *testing.T) {
	t.Parallel()

	var (
		_ workflow.SagaRegistrar = (workflow.SagaRegistrar)(nil)
		_ workflow.SagaRunner    = (workflow.SagaRunner)(nil)
		_ workflow.SagaInspector = (workflow.SagaInspector)(nil)
	)
}
