package workflow_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/workflow"
)

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	t.Run("zero/valid", func(t *testing.T) {
		t.Parallel()

		var opts workflow.Options
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate(zero) = %v, want nil", err)
		}
	})

	t.Run("valid/host_port", func(t *testing.T) {
		t.Parallel()

		opts := workflow.Options{HostPort: "localhost:7233", Namespace: "default"}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate(valid) = %v, want nil", err)
		}
	})

	t.Run("invalid/missing_port", func(t *testing.T) {
		t.Parallel()

		opts := workflow.Options{HostPort: "missing-port"}
		err := opts.Validate()
		if err == nil {
			t.Fatal("Validate(missing-port) = nil, want error")
		}

		if !errors.Is(err, workflow.ErrInvalidOptions) {
			t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
		}

		var invErr *workflow.InvalidOptionsError
		if !errors.As(err, &invErr) {
			t.Fatalf("errors.As(err, *InvalidOptionsError) = false (err = %T %v)", err, err)
		}
	})
}
