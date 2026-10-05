package google

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// TestEdgeReverseGeocodeBoundary checks the inclusive WGS84 boundaries.
func TestEdgeReverseGeocodeBoundary(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","results":[{"formatted_address":"Pole"}]}`, 200)
	g := newGeoWithServer(t, srv)
	ctx := t.Context()

	for _, pt := range []struct{ lat, lng float64 }{
		{90, 180},
		{-90, -180},
		{0, 0},
	} {
		if _, err := g.ReverseGeocode(ctx, pt.lat, pt.lng); err != nil {
			t.Errorf("ReverseGeocode(%v,%v) error = %v, want nil", pt.lat, pt.lng, err)
		}
	}
}

// TestEdgeDistanceSamePoint checks the zero-distance boundary.
func TestEdgeDistanceSamePoint(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","rows":[{"elements":[{"status":"OK","distance":{"value":0}}]}]}`, 200)
	g := newGeoWithServer(t, srv)

	got, err := g.Distance(t.Context(), geo.Point{Lat: 1, Lng: 2}, geo.Point{Lat: 1, Lng: 2})
	if err != nil {
		t.Fatalf("Distance() error = %v", err)
	}

	if got != 0 {
		t.Fatalf("Distance(same point) = %v, want 0", got)
	}
}

// TestEdgeGeocodeEmptyAddress checks that an empty address fails fast in the
// client without reaching the server.
func TestEdgeGeocodeEmptyAddress(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","results":[{"formatted_address":"X"}]}`, 200)
	g := newGeoWithServer(t, srv)

	_, err := g.Geocode(t.Context(), "")
	if err == nil {
		t.Fatal("Geocode(empty) = nil, want client validation error")
	}

	if errors.Is(err, geo.ErrNotFound) {
		t.Fatalf("Geocode(empty) = %v, want client error, not ErrNotFound", err)
	}
}

// TestEdgeGeocodeNilGeometry checks that a result lacking geometry decodes
// to the zero coordinate rather than panicking.
func TestEdgeGeocodeNilGeometry(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","results":[{"formatted_address":"NoGeo"}]}`, 200)
	g := newGeoWithServer(t, srv)

	locs, err := g.Geocode(t.Context(), "x")
	if err != nil {
		t.Fatalf("Geocode() error = %v", err)
	}

	if len(locs) != 1 || locs[0].Lat != 0 || locs[0].Lng != 0 {
		t.Fatalf("locs = %+v, want one zero-coordinate result", locs)
	}
}

// TestEdgeValidateBaseURLEmpty checks that an empty base URL is rejected.
func TestEdgeValidateBaseURLEmpty(t *testing.T) {
	t.Parallel()

	if err := validateBaseURL("", false); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("validateBaseURL(\"\") = %v, want ErrInvalidOptions", err)
	}
}

// TestEdgeDistanceBoundaryCoords checks that finite boundary coordinates are
// accepted by the distance path.
func TestEdgeDistanceBoundaryCoords(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","rows":[{"elements":[{"status":"OK","distance":{"value":20015086}}]}]}`, 200)
	g := newGeoWithServer(t, srv)

	got, err := g.Distance(t.Context(), geo.Point{Lat: 90, Lng: 180}, geo.Point{Lat: -90, Lng: -180})
	if err != nil {
		t.Fatalf("Distance(boundary) error = %v", err)
	}

	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("Distance(boundary) = %v, want finite", got)
	}
}

// TestEdgeConcurrentGeocode exercises the goroutine-safe HTTP client under
// concurrent geocoding.
func TestEdgeConcurrentGeocode(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, `{"status":"OK","results":[{"formatted_address":"X","geometry":{"location":{"lat":1,"lng":2}}}]}`, 200)
	g := newGeoWithServer(t, srv)
	ctx := t.Context()

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 10 {
				if _, err := g.Geocode(ctx, "addr"); err != nil {
					t.Errorf("Geocode() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
