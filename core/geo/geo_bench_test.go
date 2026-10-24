package geo

import (
	"testing"
	"time"
)

// BenchmarkValidCoord measures coordinate validation.
func BenchmarkValidCoord(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if !ValidCoord(51.5, -0.12) {
			b.Fatal("ValidCoord = false, want true")
		}
	}
}

// BenchmarkOptionsValidate measures option range and endpoint checks.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{
		Timeout:         time.Second,
		MaxResponseBody: 1 << 20,
		BaseURL:         "https://maps.example.com",
		Endpoint:        "https://osm.example.com",
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

// BenchmarkGeocode measures address geocoding through a stub backend.
func BenchmarkGeocode(b *testing.B) {
	g := &stubGeo{}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := g.Geocode(ctx, "London"); err != nil {
			b.Fatalf("Geocode err = %v", err)
		}
	}
}

// BenchmarkDistance measures the Geo distance dispatch through a stub.
func BenchmarkDistance(b *testing.B) {
	g := &stubGeo{}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := g.Distance(ctx, Point{Lat: 51.5, Lng: -0.12}, Point{Lat: 48.85, Lng: 2.35}); err != nil {
			b.Fatalf("Distance err = %v", err)
		}
	}
}
