package log

import (
	"io"
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/analytics/analyticstest"
)

// TestLogConformance proves the log adapter honors the
// analytics.Analytics contract via the shared conformance kit. Events
// are discarded (no network, no credentials).
func TestLogConformance(t *testing.T) {
	t.Parallel()

	analyticstest.Conformance(t, func(t *testing.T) analytics.Analytics {
		t.Helper()

		a, err := NewWithWriter(analytics.Options{}, io.Discard)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
