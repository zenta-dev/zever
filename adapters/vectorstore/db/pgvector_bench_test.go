package db

import (
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
)

// benchEmbedding is the fixed vector used by the upsert/query benches.
func benchEmbedding() []float32 {
	return []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
}

// newBenchStore opens a file-backed sqlite vector store.
func newBenchStore(b *testing.B) vectorstore.VectorStore {
	b.Helper()

	s, err := New(vectorstore.Options{DSN: filepath.Join(b.TempDir(), "vectors.db")})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// BenchmarkUpsert measures the single-vector write: dimension check, binary
// embedding encode, metadata encode, and an upsert.
func BenchmarkUpsert(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	vec := vectorstore.Vector{ID: "bench-1", Embedding: benchEmbedding(), Metadata: map[string]any{"kind": "bench"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Upsert(ctx, vec); err != nil {
			b.Fatalf("Upsert(): %v", err)
		}
	}
}

// BenchmarkUpsertBatch measures a small batch write: one upsert per vector.
func BenchmarkUpsertBatch(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	vecs := make([]vectorstore.Vector, 8)
	for i := range vecs {
		vecs[i] = vectorstore.Vector{
			ID:        "batch-" + string(rune('a'+i)),
			Embedding: benchEmbedding(),
			Metadata:  map[string]any{"i": i},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.UpsertBatch(ctx, vecs); err != nil {
			b.Fatalf("UpsertBatch(): %v", err)
		}
	}
}

// BenchmarkQuery measures the brute-force top-K scan: decode every row, score
// with cosine similarity, and retain the best in a bounded heap.
func BenchmarkQuery(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	for i := 0; i < 256; i++ {
		vec := vectorstore.Vector{
			ID:        "doc-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Embedding: benchEmbedding(),
			Metadata:  map[string]any{"i": i},
		}

		if err := s.Upsert(ctx, vec); err != nil {
			b.Fatalf("seed Upsert(): %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Query(ctx, benchEmbedding(), 10); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}
