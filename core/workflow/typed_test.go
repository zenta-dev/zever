package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

type typedSagaIn struct {
	ItemID string `json:"item_id"`
}

type typedSagaOut struct {
	ID string `json:"id"`
}

type typedCompIn struct {
	Amount int64 `json:"amount"`
}

type typedCompOut struct {
	Refunded bool `json:"refunded"`
}

func TestNewTypedStepRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	exec := workflow.StepFuncOf[typedSagaIn, typedSagaOut](func(_ context.Context, in typedSagaIn) (typedSagaOut, error) {
		return typedSagaOut{ID: in.ItemID}, nil
	})
	comp := workflow.StepFuncOf[typedCompIn, typedCompOut](func(_ context.Context, in typedCompIn) (typedCompOut, error) {
		return typedCompOut{Refunded: in.Amount > 0}, nil
	})

	step := workflow.NewTypedStep("reserve", exec, comp, false)
	got := step.AsSagaStep()

	if got.Name != "reserve" {
		t.Errorf("AsSagaStep().Name = %q, want reserve", got.Name)
	}

	if got.Pivot {
		t.Errorf("AsSagaStep().Pivot = true, want false")
	}

	if got.Execute == nil {
		t.Fatal("AsSagaStep().Execute = nil, want non-nil")
	}

	out, err := got.Execute(ctx, map[string]string{"item_id": "item-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	typedOut, ok := out.(typedSagaOut)
	if !ok {
		t.Fatalf("Execute returned %T, want typedSagaOut", out)
	}

	if typedOut.ID != "item-1" {
		t.Errorf("Execute output ID = %q, want item-1", typedOut.ID)
	}

	if got.Compensate == nil {
		t.Fatal("AsSagaStep().Compensate = nil, want non-nil")
	}

	cout, err := got.Compensate(ctx, map[string]int64{"amount": 10})
	if err != nil {
		t.Fatalf("Compensate: %v", err)
	}

	typedCOut, ok := cout.(typedCompOut)
	if !ok {
		t.Fatalf("Compensate returned %T, want typedCompOut", cout)
	}

	if !typedCOut.Refunded {
		t.Errorf("Compensate output Refunded = false, want true")
	}
}

func TestNewTypedStepWithoutCompensate(t *testing.T) {
	t.Parallel()

	exec := workflow.StepFuncOf[typedSagaIn, typedSagaOut](func(_ context.Context, in typedSagaIn) (typedSagaOut, error) {
		return typedSagaOut{ID: in.ItemID}, nil
	})

	step := workflow.NewTypedStepWithoutCompensate("charge", exec, true)
	got := step.AsSagaStep()

	if got.Name != "charge" {
		t.Errorf("AsSagaStep().Name = %q, want charge", got.Name)
	}

	if !got.Pivot {
		t.Errorf("AsSagaStep().Pivot = false, want true")
	}

	if got.Compensate != nil {
		t.Errorf("AsSagaStep().Compensate = non-nil, want nil")
	}
}

func TestNewTypedStepBadInputFails(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	exec := workflow.StepFuncOf[typedSagaIn, typedSagaOut](func(_ context.Context, in typedSagaIn) (typedSagaOut, error) {
		return typedSagaOut{ID: in.ItemID}, nil
	})
	comp := workflow.StepFuncOf[typedCompIn, typedCompOut](func(_ context.Context, _ typedCompIn) (typedCompOut, error) {
		return typedCompOut{}, nil
	})

	step := workflow.NewTypedStep("reserve", exec, comp, false)

	if _, err := step.AsSagaStep().Execute(ctx, func() {}); err == nil {
		t.Fatalf("Execute with unmarshalable input err = nil, want error")
	} else if !errors.Is(err, workflow.ErrTypedStepConvert) {
		t.Errorf("Execute err = %v, want errors.Is ErrTypedStepConvert", err)
	}
}

func TestTypedStepsToSagaSteps(t *testing.T) {
	t.Parallel()

	exec := workflow.StepFuncOf[typedSagaIn, typedSagaOut](func(_ context.Context, in typedSagaIn) (typedSagaOut, error) {
		return typedSagaOut{ID: in.ItemID}, nil
	})

	steps := workflow.TypedStepsToSagaSteps([]workflow.TypedStep{
		workflow.NewTypedStepWithoutCompensate("a", exec, false),
	})

	if len(steps) != 1 {
		t.Fatalf("len(TypedStepsToSagaSteps) = %d, want 1", len(steps))
	}

	if steps[0].Name != "a" {
		t.Errorf("steps[0].Name = %q, want a", steps[0].Name)
	}
}

func TestTypedStepConvertErrorMessage(t *testing.T) {
	t.Parallel()

	if msg := workflow.ErrTypedStepConvert.Error(); !strings.Contains(msg, "workflow:") {
		t.Errorf("ErrTypedStepConvert = %q, want workflow: prefix", msg)
	}
}
