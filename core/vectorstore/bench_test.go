package vectorstore

import (
	"testing"
)

// benchVectorAdapter registers a stub store once and returns its adapter so
// Open can be measured without the one-shot registration cost.
func benchVectorAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func benchEmbedding() []float32 {
	v := make([]float32, DefaultDimension)
	for i := range v {
		v[i] = float32(i%7) * 0.1
	}
	return v
}

func BenchmarkOpen(b *testing.B) {
	a := benchVectorAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkCosineSimilarity(b *testing.B) {
	a := benchEmbedding()
	c := benchEmbedding()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if CosineSimilarity(a, c) == 0 {
			b.Fatal("CosineSimilarity returned 0 for self-similar vectors")
		}
	}
}

func BenchmarkVectorValidate(b *testing.B) {
	vec := Vector{
		ID:        "v1",
		Embedding: benchEmbedding(),
		Metadata:  map[string]any{"source": "doc-1", "chunk": 3},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := vec.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{URL: "https://qdrant.example.com:6334", Dimension: DefaultDimension}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
