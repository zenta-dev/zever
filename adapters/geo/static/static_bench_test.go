package static

import (
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// benchStatic opens the checked-in cities fixture for benchmarks.
func benchStatic(b *testing.B) geo.Geo {
	b.Helper()

	g, err := New(geo.Options{Path: "testdata/cities.json"})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = g.Close() })

	return g
}

// BenchmarkNew measures reading and decoding the cities catalog.
func BenchmarkNew(b *testing.B) {
	opts := geo.Options{Path: "testdata/cities.json"}

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

// BenchmarkGeocodeExact measures an exact-match lookup.
func BenchmarkGeocodeExact(b *testing.B) {
	g := benchStatic(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Geocode(ctx, "London"); err != nil {
			b.Fatalf("Geocode() error = %v", err)
		}
	}
}

// BenchmarkGeocodePrefix measures a prefix-match lookup.
func BenchmarkGeocodePrefix(b *testing.B) {
	g := benchStatic(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.Geocode(ctx, "New"); err != nil {
			b.Fatalf("Geocode() error = %v", err)
		}
	}
}

// BenchmarkReverseGeocode measures a proximity lookup.
func BenchmarkReverseGeocode(b *testing.B) {
	g := benchStatic(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := g.ReverseGeocode(ctx, 51.5074, -0.1278); err != nil {
			b.Fatalf("ReverseGeocode() error = %v", err)
		}
	}
}

// BenchmarkDistance measures the pure great-circle distance path.
func BenchmarkDistance(b *testing.B) {
	g := benchStatic(b)
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
