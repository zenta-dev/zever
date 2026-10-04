package static

import (
	"errors"
	"math"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// edgeCities returns a small deterministic city table for boundary lookups.
func edgeCities() *staticGeo {
	return &staticGeo{cities: []city{
		{Name: "London", Lat: 51.5074, Lng: -0.1278},
		{Name: "Paris", Lat: 48.8566, Lng: 2.3522},
	}}
}

// TestEdgeGeocode_emptyQuery proves empty and whitespace-only queries are
// rejected as not-found rather than matching every prefix.
func TestEdgeGeocode_emptyQuery(t *testing.T) {
	t.Parallel()

	g := edgeCities()

	for _, q := range []string{"", " ", "\t\n"} {
		if _, err := g.Geocode(t.Context(), q); !errors.Is(err, geo.ErrNotFound) {
			t.Fatalf("Geocode(%q) err = %v, want ErrNotFound", q, err)
		}
	}
}

// TestEdgeReverseGeocode_coordBoundaries proves exact coordinate limits are
// valid while out-of-range and NaN values fail closed.
func TestEdgeReverseGeocode_coordBoundaries(t *testing.T) {
	t.Parallel()

	g := edgeCities()

	cases := []struct {
		name    string
		lat     float64
		lng     float64
		wantErr bool
	}{
		{"max valid", 90, 180, false},
		{"min valid", -90, -180, false},
		{"lat over", 90.1, 0, true},
		{"lng over", 0, 180.1, true},
		{"nan", math.NaN(), 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := g.ReverseGeocode(t.Context(), tc.lat, tc.lng)
			if tc.wantErr && !errors.Is(err, geo.ErrInvalidCoordinate) {
				t.Fatalf("ReverseGeocode(%v,%v) err = %v, want ErrInvalidCoordinate", tc.lat, tc.lng, err)
			}

			if !tc.wantErr && errors.Is(err, geo.ErrInvalidCoordinate) {
				t.Fatalf("ReverseGeocode(%v,%v) err = %v, want non-coordinate error", tc.lat, tc.lng, err)
			}
		})
	}
}

// TestEdgeDistance_samePointIsZero proves the distance to the same point is
// exactly zero (haversine lower clamp).
func TestEdgeDistance_samePointIsZero(t *testing.T) {
	t.Parallel()

	g := edgeCities()
	p := geo.Point{Lat: 51.5074, Lng: -0.1278}

	d, err := g.Distance(t.Context(), p, p)
	if err != nil {
		t.Fatalf("Distance: %v", err)
	}

	if d != 0 {
		t.Fatalf("Distance(same) = %v, want 0", d)
	}
}
