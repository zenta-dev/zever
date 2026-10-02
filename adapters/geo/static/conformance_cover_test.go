package static

import (
	"testing"

	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/geo/geotest"
)

// TestStaticConformance proves the static adapter honors the geo.Geo
// contract via the shared conformance kit. Each subtest gets a fresh
// instance over the checked-in cities fixture (no network).
func TestStaticConformance(t *testing.T) {
	t.Parallel()

	geotest.Conformance(t, func(t *testing.T) geo.Geo {
		t.Helper()

		g, err := New(geo.Options{Path: "testdata/cities.json"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = g.Close() })

		return g
	})
}
