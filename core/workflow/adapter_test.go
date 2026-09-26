package workflow_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

func TestAdapterString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		adapter workflow.Adapter
		want    string
	}{
		{"memory", workflow.Memory, "memory"},
		{"unknown", workflow.Adapter(""), "unknown"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := c.adapter.String(); got != c.want {
				t.Errorf("Adapter(%q).String() = %q, want %q", string(c.adapter), got, c.want)
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

	t.Run("open/bogus", func(t *testing.T) {
		t.Parallel()

		got, err := workflow.ParseAdapter("bogus")
		if err != nil {
			t.Fatalf("ParseAdapter(bogus) error = %v, want nil (open adapter)", err)
		}
		if got != workflow.Adapter("bogus") {
			t.Errorf("ParseAdapter(bogus) got = %v, want %v", got, workflow.Adapter("bogus"))
		}
	})

	t.Run("open/uppercase", func(t *testing.T) {
		t.Parallel()
		got, err := workflow.ParseAdapter("Memory")
		if err != nil {
			t.Fatalf("ParseAdapter(Memory) error = %v, want nil (open adapter)", err)
		}
		if got != workflow.Adapter("Memory") {
			t.Errorf("ParseAdapter(Memory) got = %v, want %v", got, workflow.Adapter("Memory"))
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
