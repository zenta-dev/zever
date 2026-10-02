package paddle

import (
	"testing"

	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/billing/billingtest"
)

// TestPaddleConformance proves the paddle adapter honors the
// billing.Billing contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live Paddle API key and
// network access. The kit's lifecycle assertions match the stub
// adapter's; provider mapping coverage lives in the adapter's own
// tests. Re-enable with a test API key.
func TestPaddleConformance(t *testing.T) {
	t.Skip("needs live Paddle API key and network")

	billingtest.Conformance(t, func(t *testing.T) billing.Billing {
		t.Helper()

		b, err := New(billing.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
