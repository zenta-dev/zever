package geo

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := geoFreshAdapter()
	if err := Register(a, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := geoFreshAdapter()
	if err := Register(a, func(Options) (Geo, error) { return &stubGeo{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkDistance(b *testing.B) {
	g := &stubGeo{}
	from := Point{Lat: 51.5, Lng: -0.1}
	to := Point{Lat: 48.85, Lng: 2.35}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := g.Distance(b.Context(), from, to); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidCoord(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ValidCoord(51.5, -0.1)
	}
}
