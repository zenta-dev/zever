package otlp

import (
	"testing"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/observability/observabilitytest"
)

// TestOTLPConformance proves the otlp adapter honors the
// observability.Provider contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live OTLP collector
// (endpoint + TLS) and network access. The kit's call-shape
// assertions match the stdout adapter's; export coverage lives in
// the adapter's own tests. Re-enable with a loopback collector.
func TestOTLPConformance(t *testing.T) {
	t.Skip("needs live OTLP collector and network")

	observabilitytest.Conformance(t, func(t *testing.T) observability.Provider {
		t.Helper()

		p, err := New(observability.Options{ServiceName: "kit", Endpoint: "localhost:4317", Insecure: true})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = p.Shutdown(t.Context()) })

		return p
	})
}
