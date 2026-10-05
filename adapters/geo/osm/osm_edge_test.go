package osm

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// edgeGeo builds an OSM adapter over handler with pacing disabled.
func edgeGeo(t *testing.T, handler http.HandlerFunc) *osmGeo {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "edge/1.0", AllowInsecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	o, ok := g.(*osmGeo)
	if !ok {
		t.Fatalf("New() returned %T, want *osmGeo", g)
	}

	o.minInterval = 0
	t.Cleanup(func() { _ = o.Close() })

	return o
}

// TestEdgeDistanceSamePoint checks the zero-distance boundary.
func TestEdgeDistanceSamePoint(t *testing.T) {
	t.Parallel()

	o := &osmGeo{}

	got, err := o.Distance(t.Context(), geo.Point{Lat: 1, Lng: 2}, geo.Point{Lat: 1, Lng: 2})
	if err != nil {
		t.Fatalf("Distance() error = %v", err)
	}

	if got != 0 {
		t.Fatalf("Distance(same point) = %v, want 0", got)
	}
}

// TestEdgeDistanceBoundaryCoords checks finite boundary coordinates.
func TestEdgeDistanceBoundaryCoords(t *testing.T) {
	t.Parallel()

	o := &osmGeo{}

	got, err := o.Distance(t.Context(), geo.Point{Lat: 90, Lng: 180}, geo.Point{Lat: -90, Lng: -180})
	if err != nil {
		t.Fatalf("Distance(boundary) error = %v", err)
	}

	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("Distance(boundary) = %v, want finite", got)
	}
}

// TestEdgeHaversineIdentical checks the zero kernel.
func TestEdgeHaversineIdentical(t *testing.T) {
	t.Parallel()

	if got := haversine(12.34, 56.78, 12.34, 56.78); got != 0 {
		t.Fatalf("haversine(same) = %v, want 0", got)
	}
}

// TestEdgeGeocodeEmptyAddress checks that an empty address yields the
// server's empty-result error.
func TestEdgeGeocodeEmptyAddress(t *testing.T) {
	t.Parallel()

	o := edgeGeo(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	if _, err := o.Geocode(t.Context(), ""); !errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("Geocode(empty) = %v, want ErrNotFound", err)
	}
}

// TestEdgeReverseGeocodeBoundaryCoords checks that boundary coordinates are
// accepted and decoded.
func TestEdgeReverseGeocodeBoundaryCoords(t *testing.T) {
	t.Parallel()

	o := edgeGeo(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"place_id":1,"lat":"90","lon":"180","display_name":"Pole","address":{"country":"X"}}`))
	})

	addrs, err := o.ReverseGeocode(t.Context(), 90, 180)
	if err != nil {
		t.Fatalf("ReverseGeocode(boundary) error = %v", err)
	}

	if len(addrs) != 1 || addrs[0].Formatted != "Pole" {
		t.Fatalf("addrs = %+v, want one Pole result", addrs)
	}
}

// TestEdgeBuildURLNoParams checks URL assembly without a query string.
func TestEdgeBuildURLNoParams(t *testing.T) {
	t.Parallel()

	o := &osmGeo{endpoint: "https://example.com"}

	if got := o.buildURL("/search", nil); got != "https://example.com/search" {
		t.Fatalf("buildURL(no params) = %q, want %q", got, "https://example.com/search")
	}
}

// TestEdgeConcurrentDistance exercises the pure distance path concurrently.
func TestEdgeConcurrentDistance(t *testing.T) {
	t.Parallel()

	o := &osmGeo{}
	ctx := t.Context()
	from := geo.Point{Lat: 40.7, Lng: -74.0}
	to := geo.Point{Lat: 51.5, Lng: -0.1}

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 100 {
				if _, err := o.Distance(ctx, from, to); err != nil {
					t.Errorf("Distance() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
