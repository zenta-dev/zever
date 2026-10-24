package vectorstore

import (
	"testing"
)

// benchEmbedding builds a deterministic test vector.
func benchEmbedding() []float32 {
	v := make([]float32, DefaultDimension)
	for i := range v {
		v[i] = float32(i%7) * 0.1
	}
	return v
}

func BenchmarkCosineSimilarity(b *testing.B) {
	a := benchEmbedding()
	c := benchEmbedding()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
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

	for b.Loop() {
		if err := vec.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{URL: "https://qdrant.example.com:6334", Dimension: DefaultDimension}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
