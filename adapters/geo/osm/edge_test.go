package osm

import (
	"errors"
	"math"
	"net/url"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// TestEdgeDistance_invalidCoords proves each out-of-range coordinate fails
// closed before any computation.
func TestEdgeDistance_invalidCoords(t *testing.T) {
	t.Parallel()

	g := &osmGeo{}
	valid := geo.Point{Lat: 51.5074, Lng: -0.1278}

	cases := []struct {
		name string
		from geo.Point
		to   geo.Point
	}{
		{"lat over", geo.Point{Lat: 90.5}, valid},
		{"lat under", geo.Point{Lat: -90.5}, valid},
		{"lng over", valid, geo.Point{Lat: 0, Lng: 180.5}},
		{"nan", geo.Point{Lat: math.NaN()}, valid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := g.Distance(t.Context(), tc.from, tc.to); !errors.Is(err, geo.ErrInvalidCoordinate) {
				t.Fatalf("Distance err = %v, want ErrInvalidCoordinate", err)
			}
		})
	}
}

// TestEdgeBuildURL_noParams proves the URL builder appends no query string
// when params are empty or nil.
func TestEdgeBuildURL_noParams(t *testing.T) {
	t.Parallel()

	g := &osmGeo{endpoint: "https://example.com"}

	for _, params := range []url.Values{nil, {}} {
		if got := g.buildURL("/search", params); got != "https://example.com/search" {
			t.Fatalf("buildURL = %q, want no query string", got)
		}
	}
}

// TestEdgeNew_whitespaceUserAgent proves a whitespace-only user agent is
// treated as missing.
func TestEdgeNew_whitespaceUserAgent(t *testing.T) {
	t.Parallel()

	if _, err := New(geo.Options{UserAgent: "   "}); !errors.Is(err, geo.ErrInvalidOptions) {
		t.Fatalf("New(whitespace UA) err = %v, want ErrInvalidOptions", err)
	}
}
