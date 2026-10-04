package qdrant

import (
	"testing"

	"github.com/zenta-dev/zever/core/vectorstore"
)

// benchEmbedding is the fixed vector used by the upsert/query benches.
func benchEmbedding() []float32 {
	return []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
}

// newBenchStore builds a Store over an in-process stub client, so benchmarks
// exercise request construction, point encoding, and response decoding with
// no network.
func newBenchStore(b *testing.B) *Store {
	b.Helper()

	return newStore(newStub(), 0)
}

// BenchmarkUpsert measures building one point (payload map, deterministic
// point ID, dense vector) and the stub upsert round trip.
func BenchmarkUpsert(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	vec := vectorstore.Vector{ID: "bench-1", Embedding: benchEmbedding(), Metadata: map[string]any{"kind": "bench"}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := s.Upsert(ctx, vec); err != nil {
			b.Fatalf("Upsert(): %v", err)
		}
	}
}

// BenchmarkUpsertBatch measures a small batch upsert: one UpsertPoints call
// carrying every point.
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

	for i := 0; i < b.N; i++ {
		if err := s.UpsertBatch(ctx, vecs); err != nil {
			b.Fatalf("UpsertBatch(): %v", err)
		}
	}
}

// BenchmarkQuery measures building the dense query and decoding scored points
// (payload extraction) from the stub client.
func BenchmarkQuery(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	for i := 0; i < 64; i++ {
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

	for i := 0; i < b.N; i++ {
		if _, err := s.Query(ctx, benchEmbedding(), 10); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}

// BenchmarkPointID measures deterministic point-ID generation, which hashes
// non-UUID ids with SHA-256 and sets RFC 4122 bits.
func BenchmarkPointID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if id := pointID("benchmark-document-id"); id == nil {
			b.Fatal("pointID() returned nil")
		}
	}
}
