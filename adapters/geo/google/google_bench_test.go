package google

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// stubGeo is a no-op geo.Geo used to isolate registry benchmarks.
type stubGeo struct{}

func (stubGeo) Geocode(context.Context, string) ([]geo.Location, error) { return nil, nil }
func (stubGeo) ReverseGeocode(context.Context, float64, float64) ([]geo.Address, error) {
	return nil, nil
}
func (stubGeo) Distance(context.Context, geo.Point, geo.Point) (float64, error) { return 0, nil }
func (stubGeo) Close() error                                                    { return nil }

const benchGeocodeBody = `{"status":"OK","results":[` +
	`{"formatted_address":"A","geometry":{"location":{"lat":1.5,"lng":2.5}}},` +
	`{"formatted_address":"B","geometry":{"location":{"lat":3.5,"lng":4.5}}}]}`

const benchDistanceBody = `{"status":"OK","rows":[{"elements":[` +
	`{"status":"OK","distance":{"text":"12.5 km","value":12535}}]}]}`

// benchGeo builds a Google adapter pointed at a stub HTTP server (loopback,
// no real network).
func benchGeo(b *testing.B) geo.Geo {
	b.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "distancematrix") {
			_, _ = io.WriteString(w, benchDistanceBody)

			return
		}

		_, _ = io.WriteString(w, benchGeocodeBody)
	}))
	b.Cleanup(srv.Close)

	g, err := New(geo.Options{APIKey: "fake-key", BaseURL: srv.URL, AllowInsecure: true, AllowPrivate: true})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = g.Close() })

	return g
}

// BenchmarkNew measures constructing a Google Maps client.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		g, err := New(geo.Options{APIKey: "fake"})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = g.Close()
	}
}

// BenchmarkValidateBaseURL measures base URL validation.
func BenchmarkValidateBaseURL(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := validateBaseURL("https://maps.googleapis.com", false); err != nil {
			b.Fatalf("validateBaseURL() error = %v", err)
		}
	}
}

// BenchmarkGeocode measures a geocode round trip against the stub server.
func BenchmarkGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Geocode(ctx, "1600 Amphitheatre Parkway"); err != nil {
			b.Fatalf("Geocode() error = %v", err)
		}
	}
}

// BenchmarkReverseGeocode measures a reverse-geocode round trip against the
// stub server.
func BenchmarkReverseGeocode(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.ReverseGeocode(ctx, 37.422, -122.084); err != nil {
			b.Fatalf("ReverseGeocode() error = %v", err)
		}
	}
}

// BenchmarkDistance measures a distance-matrix round trip against the stub
// server.
func BenchmarkDistance(b *testing.B) {
	g := benchGeo(b)
	ctx := b.Context()
	from := geo.Point{Lat: 1.3, Lng: 103.7}
	to := geo.Point{Lat: 1.2, Lng: 103.8}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Distance(ctx, from, to); err != nil {
			b.Fatalf("Distance() error = %v", err)
		}
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
