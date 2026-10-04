package flag

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
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
	if err := Register(a, func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
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

func BenchmarkValidateKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ValidateKey("checkout.new_flow"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetJSON(b *testing.B) {
	f := &jsonStubFlag{values: map[string]any{"app": `{"port":8080}`}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := GetJSON(b.Context(), f, "app", jsonConfig{}); err != nil {
			b.Fatal(err)
		}
	}
}
