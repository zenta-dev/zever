package osm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

const benchSearchBody = `[{"place_id":1,"lat":"40.7128","lon":"-74.0060",` +
	`"display_name":"New York, NY","address":{"city":"New York","country":"USA"}}]`

const benchReverseBody = `{"place_id":1,"lat":"40.7128","lon":"-74.0060",` +
	`"display_name":"New York, NY","address":{"city":"New York","country":"USA"}}`

// benchGeo builds an OSM adapter pointed at a stub HTTP server with request
// pacing disabled so the benchmark measures parsing, not the rate limiter.
func benchGeo(b *testing.B) *osmGeo {
	b.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasPrefix(r.URL.Path, "/reverse") {
			_, _ = io.WriteString(w, benchReverseBody)

			return
		}

		_, _ = io.WriteString(w, benchSearchBody)
	}))
	b.Cleanup(srv.Close)

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "bench/1.0", AllowInsecure: true, AllowPrivate: true})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	o, ok := g.(*osmGeo)
	if !ok {
		b.Fatalf("New() returned %T, want *osmGeo", g)
	}

	o.minInterval = 0

	b.Cleanup(func() { _ = o.Close() })

	return o
}

// BenchmarkNew measures constructing an OSM adapter.
func BenchmarkNew(b *testing.B) {
	opts := geo.Options{UserAgent: "bench/1.0"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		g, err := New(opts)
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = g.Close()
	}
}

// BenchmarkGeocode measures a search round trip plus decode against the stub
// server.
func BenchmarkGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Geocode(ctx, "New York"); err != nil {
			b.Fatalf("Geocode() error = %v", err)
		}
	}
}

// BenchmarkReverseGeocode measures a reverse round trip plus decode against
// the stub server.
func BenchmarkReverseGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.ReverseGeocode(ctx, 40.7128, -74.0060); err != nil {
			b.Fatalf("ReverseGeocode() error = %v", err)
		}
	}
}

// BenchmarkDistance measures the pure great-circle distance path.
func BenchmarkDistance(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()
	from := geo.Point{Lat: 40.7128, Lng: -74.0060}
	to := geo.Point{Lat: 51.5074, Lng: -0.1278}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Distance(ctx, from, to); err != nil {
			b.Fatalf("Distance() error = %v", err)
		}
	}
}

// BenchmarkHaversine measures the haversine kernel directly.
func BenchmarkHaversine(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = haversine(40.7128, -74.0060, 51.5074, -0.1278)
	}
}

// BenchmarkBuildURL measures URL assembly.
func BenchmarkBuildURL(b *testing.B) {
	g := benchGeo(b)
	params := url.Values{"q": {"New York"}, "format": {"jsonv2"}, "limit": {"10"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = g.buildURL("/search", params)
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		g, openErr := New(geo.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := g.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
