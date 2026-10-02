package stripe

import (
	"testing"

	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/billing/billingtest"
)

// TestStripeConformance proves the stripe adapter honors the
// billing.Billing contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live Stripe secret key and
// network access. The kit's lifecycle assertions match the stub
// adapter's; provider mapping coverage lives in the adapter's own
// tests. Re-enable with a test secret key.
func TestStripeConformance(t *testing.T) {
	t.Skip("needs live Stripe secret key and network")

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
