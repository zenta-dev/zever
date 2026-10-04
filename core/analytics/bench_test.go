package analytics

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Analytics, error) { return &stubAnalytics{}, nil }); err != nil {
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
	a := freshAdapter()
	if err := Register(a, func(Options) (Analytics, error) { return &stubAnalytics{}, nil }); err != nil {
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

func BenchmarkTrack(b *testing.B) {
	stub := &stubAnalytics{}
	props := map[string]any{"plan": "pro", "count": 3}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := stub.Track(b.Context(), "event", props); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePropertiesSize(b *testing.B) {
	props := map[string]any{"plan": "pro", "count": 3, "active": true}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ValidatePropertiesSize(props, DefaultMaxPropertiesBytes); err != nil {
			b.Fatal(err)
		}
	}
}
