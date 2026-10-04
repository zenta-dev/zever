package static

import (
	"testing"

	"github.com/zenta-dev/zever/core/geo"
)

// benchGeo builds a static adapter over a small in-memory city table.
func benchGeo() *staticGeo {
	return &staticGeo{cities: []city{
		{Name: "London", Lat: 51.5074, Lng: -0.1278},
		{Name: "Londonderry", Lat: 54.9966, Lng: -7.3086},
		{Name: "Paris", Lat: 48.8566, Lng: 2.3522},
		{Name: "Berlin", Lat: 52.5200, Lng: 13.4050},
		{Name: "Madrid", Lat: 40.4168, Lng: -3.7038},
		{Name: "Rome", Lat: 41.9028, Lng: 12.4964},
		{Name: "Tokyo", Lat: 35.6762, Lng: 139.6503},
		{Name: "New York", Lat: 40.7128, Lng: -74.0060},
	}}
}

// BenchmarkGeocode measures the linear scan for an exact match.
func BenchmarkGeocode(b *testing.B) {
	g := benchGeo()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Geocode(ctx, "Paris"); err != nil {
			b.Fatalf("Geocode: %v", err)
		}
	}
}

// BenchmarkReverseGeocode measures the haversine scan for nearby cities.
func BenchmarkReverseGeocode(b *testing.B) {
	g := benchGeo()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.ReverseGeocode(ctx, 51.5074, -0.1278); err != nil {
			b.Fatalf("ReverseGeocode: %v", err)
		}
	}
}

// BenchmarkDistance measures coordinate validation plus haversine.
func BenchmarkDistance(b *testing.B) {
	g := benchGeo()
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
