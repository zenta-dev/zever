package flagtest_test

import (
	"testing"

	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/flag/flagtest"
)

// TestConformanceStatic proves the kit passes against the static adapter
// with an empty flag set.
func TestConformanceStatic(t *testing.T) {
	t.Parallel()

	flagtest.Conformance(t, func(t *testing.T) flag.Flag {
		t.Helper()

		f, err := flagstatic.New(flag.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = f.Close() })

		return f
	})
}
