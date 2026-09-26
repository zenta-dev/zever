package workflow_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrNilFactory":     {workflow.ErrNilFactory.Error(), "workflow: nil factory"},
		"ErrDuplicate":      {workflow.ErrDuplicate.Error(), "workflow: duplicate registration"},
		"ErrUnknownAdapter": {workflow.ErrUnknownAdapter.Error(), "workflow: unknown adapter"},
		"ErrInvalidAdapter": {workflow.ErrInvalidAdapter.Error(), "workflow: invalid adapter"},
		"ErrInvalidOptions": {workflow.ErrInvalidOptions.Error(), "workflow: invalid options"},
		"ErrUnknownRun":     {workflow.ErrUnknownRun.Error(), "workflow: unknown run"},
		"ErrRunCompleted":   {workflow.ErrRunCompleted.Error(), "workflow: run completed"},
		"ErrPendingFull":    {workflow.ErrPendingFull.Error(), "workflow: pending full"},
		"ErrUnknownStep":    {workflow.ErrUnknownStep.Error(), "workflow: unknown step"},
		"ErrDuplicateRun":   {workflow.ErrDuplicateRun.Error(), "workflow: duplicate run"},
		"ErrUnknownQuery":   {workflow.ErrUnknownQuery.Error(), "workflow: unknown query"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestTypedErrorsUnwrap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{"duplicate", &workflow.DuplicateAdapterError{Adapter: workflow.Memory}, workflow.ErrDuplicate},
		{"unknown_adapter", &workflow.UnknownAdapterError{Adapter: workflow.Memory}, workflow.ErrUnknownAdapter},
		{"invalid_adapter", &workflow.InvalidAdapterError{Adapter: "bogus"}, workflow.ErrInvalidAdapter},
		{"unknown_run", &workflow.UnknownRunError{RunID: "r1"}, workflow.ErrUnknownRun},
		{"run_completed", &workflow.RunCompletedError{RunID: "r1"}, workflow.ErrRunCompleted},
		{"pending_full", &workflow.PendingFullError{RunID: "r1"}, workflow.ErrPendingFull},
		{"unknown_step", &workflow.UnknownStepError{Step: "s1"}, workflow.ErrUnknownStep},
		{"duplicate_run", &workflow.DuplicateRunError{RunID: "r1"}, workflow.ErrDuplicateRun},
		{"unknown_query", &workflow.UnknownQueryError{Query: "q1"}, workflow.ErrUnknownQuery},
		{"invalid_options", &workflow.InvalidOptionsError{Reason: "bad"}, workflow.ErrInvalidOptions},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(c.err, c.want) {
				t.Errorf("errors.Is(%T, %v) = false (err = %v)", c.err, c.want, c.err)
			}

			if c.err.Error() == "" {
				t.Errorf("%T.Error() is empty", c.err)
			}
		})
	}
}

func TestTypedErrorMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"unknown_run", workflow.UnknownRunError{RunID: "r1"}, `workflow: unknown run: "r1"`},
		{"run_completed", workflow.RunCompletedError{RunID: "r1"}, `workflow: run "r1" has completed`},
		{"pending_full", workflow.PendingFullError{RunID: "r1"}, `workflow: run "r1" pending buffer full`},
		{"unknown_step", workflow.UnknownStepError{Step: "s1"}, `workflow: unknown step: "s1"`},
		{"duplicate_run", workflow.DuplicateRunError{RunID: "r1"}, `workflow: run "r1" already started (duplicate workflow ID)`},
		{"unknown_query", workflow.UnknownQueryError{Query: "q1"}, `workflow: unknown query: "q1"`},
		{"invalid_options", workflow.InvalidOptionsError{Reason: "bad host"}, "workflow: invalid options: bad host"},
		{"invalid_adapter", workflow.InvalidAdapterError{Adapter: "bogus"}, `workflow: invalid adapter: "bogus"`},
		{"duplicate", workflow.DuplicateAdapterError{Adapter: workflow.Memory}, "workflow: duplicate registration: memory"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := c.err.Error(); got != c.want {
				t.Errorf("%T.Error() = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

func TestTypedErrorsAs(t *testing.T) {
	t.Parallel()

	t.Run("unknown_run", func(t *testing.T) {
		t.Parallel()

		err := &workflow.UnknownRunError{RunID: "r1"}

		var target *workflow.UnknownRunError
		if !errors.As(err, &target) {
			t.Fatalf("errors.As(%v, *UnknownRunError) = false", err)
		}

		if target.RunID != "r1" {
			t.Errorf("UnknownRunError.RunID = %q, want %q", target.RunID, "r1")
		}
	})

	t.Run("invalid_options", func(t *testing.T) {
		t.Parallel()

		err := &workflow.InvalidOptionsError{Reason: "bad"}

		var target *workflow.InvalidOptionsError
		if !errors.As(err, &target) {
			t.Fatalf("errors.As(%v, *InvalidOptionsError) = false", err)
		}

		if target.Reason != "bad" {
			t.Errorf("InvalidOptionsError.Reason = %q, want %q", target.Reason, "bad")
		}
	})
}
