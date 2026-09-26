package workflow_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/workflow"
)

func TestAdapterString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		adapter workflow.Adapter
		want    string
	}{
		{"memory", workflow.Memory, "memory"},
		{"unknown", workflow.Adapter(99), "unknown"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := c.adapter.String(); got != c.want {
				t.Errorf("Adapter(%d).String() = %q, want %q", int(c.adapter), got, c.want)
			}
		})
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	t.Run("valid/memory", func(t *testing.T) {
		t.Parallel()

		got, err := workflow.ParseAdapter("memory")
		if err != nil {
			t.Fatalf("ParseAdapter(memory) err = %v, want nil", err)
		}

		if got != workflow.Memory {
			t.Errorf("ParseAdapter(memory) = %v, want %v", got, workflow.Memory)
		}
	})

	t.Run("invalid/bogus", func(t *testing.T) {
		t.Parallel()

		got, err := workflow.ParseAdapter("bogus")
		if err == nil {
			t.Fatal("ParseAdapter(bogus) = nil, want error")
		}

		if got != workflow.Memory {
			t.Errorf("ParseAdapter(bogus) got = %v, want %v", got, workflow.Memory)
		}

		if !errors.Is(err, workflow.ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}

		var invErr *workflow.InvalidAdapterError
		if !errors.As(err, &invErr) {
			t.Fatalf("errors.As(err, *InvalidAdapterError) = false (err = %T %v)", err, err)
		}

		if invErr.Adapter != "bogus" {
			t.Errorf("InvalidAdapterError.Adapter = %q, want %q", invErr.Adapter, "bogus")
		}
	})

	t.Run("invalid/uppercase", func(t *testing.T) {
		t.Parallel()

		_, err := workflow.ParseAdapter("Memory")
		if err == nil {
			t.Fatal("ParseAdapter(Memory) = nil, want error")
		}

		if !errors.Is(err, workflow.ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		t.Parallel()

		a, err := workflow.ParseAdapter(workflow.Memory.String())
		if err != nil {
			t.Fatalf("ParseAdapter(%q) err = %v", workflow.Memory.String(), err)
		}

		if a != workflow.Memory {
			t.Errorf("roundtrip = %v, want %v", a, workflow.Memory)
		}
	})
}
