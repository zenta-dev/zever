package osm

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// benchGeocodeServer serves a fixed Nominatim search response on loopback.
func benchGeocodeServer(b *testing.B) *httptest.Server {
	b.Helper()

	const body = `[{"place_id":1,"lat":"48.8566","lon":"2.3522","display_name":"Paris, France"},` +
		`{"place_id":2,"lat":"48.85","lon":"2.35","display_name":"Paris, TX"}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))

	b.Cleanup(srv.Close)

	return srv
}

// benchGeo builds an osmGeo with pacing disabled so benchmarks are not gated
// by the one-request-per-second Nominatim policy.
func benchGeo(b *testing.B) *osmGeo {
	b.Helper()

	srv := benchGeocodeServer(b)
	u, err := url.Parse(srv.URL)
	if err != nil {
		b.Fatalf("url.Parse: %v", err)
	}

	return &osmGeo{
		endpoint:  srv.URL,
		baseURL:   u,
		client:    srv.Client(),
		userAgent: "zever-bench",
		maxBody:   defaultMaxBody,
	}
}

// BenchmarkDistance measures coordinate validation plus haversine (no I/O).
func BenchmarkDistance(b *testing.B) {
	g := &osmGeo{}
	ctx := b.Context()
	from := geo.Point{Lat: 51.5074, Lng: -0.1278}
	to := geo.Point{Lat: 48.8566, Lng: 2.3522}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Distance(ctx, from, to); err != nil {
			b.Fatalf("Distance: %v", err)
		}
	}
}

// BenchmarkBuildURL measures query construction for a search request.
func BenchmarkBuildURL(b *testing.B) {
	g := &osmGeo{endpoint: "https://nominatim.openstreetmap.org"}
	params := url.Values{"q": {"Paris"}, "format": {"jsonv2"}, "addressdetails": {"1"}, "limit": {"10"}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = g.buildURL("/search", params)
	}
}

// BenchmarkGeocode measures the request, decode, and coordinate-conversion
// path against an in-process stub server.
func BenchmarkGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Geocode(ctx, "Paris"); err != nil {
			b.Fatalf("Geocode: %v", err)
		}
	}
}
