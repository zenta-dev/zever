package google

import (
	"testing"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/geo/geotest"
)

// TestGoogleConformance proves the google adapter honors the geo.Geo
// contract via the shared conformance kit.
//
// Currently skipped: the adapter needs a live Google Maps API key
// and network access (Distance calls DistanceMatrix even for
// identical points). The kit's sentinel and same-point assertions
// match the static adapter's; provider mapping coverage lives in the
// adapter's own httptest tests. Re-enable with a test API key.
func TestGoogleConformance(t *testing.T) {
	t.Skip("needs live Google Maps API key and network")

	geotest.Conformance(t, func(t *testing.T) geo.Geo {
		t.Helper()

		g, err := New(geo.Options{APIKey: "kit-test-key"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = g.Close() })

		return g
	})
}
