package geo

import (
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := Register(geoFreshAdapter(), func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

// BenchmarkOpen measures validated registry lookup plus the factory call.
func BenchmarkOpen(b *testing.B) {
	adapter := geoFreshAdapter()
	if err := Register(adapter, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Open(adapter, Options{Timeout: time.Second}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

// BenchmarkValidCoord measures coordinate range and finiteness validation.
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

// BenchmarkAdapterString measures canonical adapter naming.
func BenchmarkAdapterString(b *testing.B) {
	a := Google

	b.ReportAllocs()

	for b.Loop() {
		_ = a.String()
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := ParseAdapter("google"); err != nil {
			b.Fatalf("ParseAdapter err = %v", err)
		}
	}
}

// BenchmarkGeocode measures the Geo contract dispatch through a stub.
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
