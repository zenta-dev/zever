package ai

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
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
	if err := Register(a, func(Options) (AI, error) { return &stubAI{}, nil }); err != nil {
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

func BenchmarkGenerate(b *testing.B) {
	stub := &stubAI{}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := stub.Generate(b.Context(), "model-x", nil, GenerateOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
