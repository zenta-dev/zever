package single

import (
	"testing"

	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/tenant/tenanttest"
)

// TestSingleConformance proves the single adapter honors the
// tenant.Tenant contract via the shared conformance kit. The fixed
// ID backend returns its configured ID for the kit fixture meta.
func TestSingleConformance(t *testing.T) {
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
