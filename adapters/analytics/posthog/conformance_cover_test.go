package posthog

import (
	"testing"

	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/analytics/analyticstest"
)

// TestPosthogConformance proves the posthog adapter honors the
// analytics.Analytics contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live PostHog write key and
// network access. The kit's Track/Identify/Group assertions match
// the log adapter's; provider mapping coverage lives in the
// adapter's own fake-client tests. Re-enable with a test write key.
func TestPosthogConformance(t *testing.T) {
	t.Skip("needs live PostHog write key and network")

	analyticstest.Conformance(t, func(t *testing.T) analytics.Analytics {
		t.Helper()

		a, err := New(analytics.Options{APIKey: "kit-test-key"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = a.Close() })

		return a
	})
}
