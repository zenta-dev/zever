package header

import (
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/tenant/tenanttest"
)

// TestHeaderConformance proves the header adapter honors the
// tenant.Tenant contract via the shared conformance kit. Resolution
// reads the kit fixture header (no network).
func TestHeaderConformance(t *testing.T) {
	t.Parallel()

	tenanttest.Conformance(t, func(t *testing.T) tenant.Tenant {
		t.Helper()

		b, err := New(tenant.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
