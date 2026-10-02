package static

import (
	"testing"

	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/flag/flagtest"
)

// TestStaticConformance proves the static adapter honors the flag.Flag
// fallback contract via the shared conformance kit. Each subtest gets
// a fresh empty flag set (no path, no file, no network); seeded-value
// coverage lives in the adapter's own tests.
func TestStaticConformance(t *testing.T) {
	flagtest.Conformance(t, func(t *testing.T) flag.Flag {
		t.Helper()

		f, err := New(flag.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = f.Close() })

		return f
	})
}
