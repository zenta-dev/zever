package analyticstest_test

import (
	"io"
	"testing"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/analytics/analyticstest"
)

// TestConformanceLog proves the kit passes against the log adapter.
func TestConformanceLog(t *testing.T) {
	t.Parallel()

	analyticstest.Conformance(t, func(t *testing.T) analytics.Analytics {
		t.Helper()

		a, err := analyticslog.NewWithWriter(analytics.Options{}, io.Discard)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
