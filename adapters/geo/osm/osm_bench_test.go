package osm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run so
// Register never collides with a previously registered factory.
func benchFreshAdapter() geo.Adapter {
	return geo.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

// stubGeo is a no-op geo.Geo used to isolate registry benchmarks.
type stubGeo struct{}

func (stubGeo) Geocode(context.Context, string) ([]geo.Location, error) { return nil, nil }
func (stubGeo) ReverseGeocode(context.Context, float64, float64) ([]geo.Address, error) {
	return nil, nil
}
func (stubGeo) Distance(context.Context, geo.Point, geo.Point) (float64, error) { return 0, nil }
func (stubGeo) Close() error                                                    { return nil }

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

	g, err := New(geo.Options{Endpoint: srv.URL, UserAgent: "bench/1.0", AllowInsecure: true})
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

// BenchmarkRegister measures registering a factory into the geo registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		err := geo.Register(benchFreshAdapter(), func(geo.Options) (geo.Geo, error) {
			return stubGeo{}, nil
		})
		if err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()

	err := geo.Register(a, func(geo.Options) (geo.Geo, error) { return stubGeo{}, nil })
	if err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		g, openErr := geo.Open(a, geo.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := g.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
