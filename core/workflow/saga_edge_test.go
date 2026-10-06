package workflow_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func TestSagaSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrUnknownSaga":            {workflow.ErrUnknownSaga.Error(), "workflow: unknown saga"},
		"ErrSagaNotFound":           {workflow.ErrSagaNotFound.Error(), "workflow: saga not found"},
		"ErrSagaStepFailed":         {workflow.ErrSagaStepFailed.Error(), "workflow: saga step failed"},
		"ErrSagaCompensationFailed": {workflow.ErrSagaCompensationFailed.Error(), "workflow: saga compensation failed"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestSagaSentinelUnwrap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"unknown_saga", fmt.Errorf("run: %w", workflow.ErrUnknownSaga), workflow.ErrUnknownSaga},
		{"not_found", fmt.Errorf("status: %w", workflow.ErrSagaNotFound), workflow.ErrSagaNotFound},
		{"step_failed", fmt.Errorf("step 2: %w", workflow.ErrSagaStepFailed), workflow.ErrSagaStepFailed},
		{"compensation_failed", fmt.Errorf("comp: %w", workflow.ErrSagaCompensationFailed), workflow.ErrSagaCompensationFailed},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(c.err, c.want) {
				t.Errorf("errors.Is(%v, %v) = false", c.err, c.want)
			}
		})
	}
}

func TestSagaSentinelsDistinct(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		workflow.ErrUnknownSaga,
		workflow.ErrSagaNotFound,
		workflow.ErrSagaStepFailed,
		workflow.ErrSagaCompensationFailed,
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel %d unwraps sentinel %d", i, j)
			}
		}
	}
}
