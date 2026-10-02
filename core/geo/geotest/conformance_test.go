package geotest_test

import (
	"testing"

	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/geo/geotest"
)

// TestConformanceStatic proves the kit passes against the static adapter.
func TestConformanceStatic(t *testing.T) {
	t.Parallel()

	geotest.Conformance(t, func(t *testing.T) geo.Geo {
		t.Helper()

		g, err := geostatic.New(geo.Options{Path: "testdata/cities.json"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = g.Close() })

		return g
	})
}
