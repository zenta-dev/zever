package db

import (
	"testing"

	"github.com/zenta-dev/zever/core/search"
)

// BenchmarkCoerceMeta measures metadata cell coercion.
func BenchmarkCoerceMeta(b *testing.B) {
	raw := []byte(`{"content":"bench"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := coerceMeta(raw); err != nil {
			b.Fatalf("coerceMeta(): %v", err)
		}
	}
}

// BenchmarkDecodeHitMeta measures hit metadata decode.
func BenchmarkDecodeHitMeta(b *testing.B) {
	raw := []byte(`{"content":"bench"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := decodeHitMeta(raw); err != nil {
			b.Fatalf("decodeHitMeta(): %v", err)
		}
	}
}

// BenchmarkBuildMatch measures FTS5 MATCH sanitizing.
func BenchmarkBuildMatch(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = buildMatch("the quick brown fox")
	}
}

// BenchmarkBuildSearchQueries measures postgres query rendering.
func BenchmarkBuildSearchQueries(b *testing.B) {
	filters := map[string]string{"index": "idx"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _, _ = buildSearchQueries("quick fox", filters, 10, 0)
	}
}

// BenchmarkMatchWhere measures postgres WHERE rendering.
func BenchmarkMatchWhere(b *testing.B) {
	filters := map[string]string{"index": "idx"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = matchWhere("quick fox", filters)
	}
}

// BenchmarkEncodeDocumentMetadata measures document metadata encode.
func BenchmarkEncodeDocumentMetadata(b *testing.B) {
	doc := search.Document{ID: "d", Index: "i", Content: "the quick brown fox", Metadata: map[string]any{"k": 1}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := encodeDocumentMetadata(doc); err != nil {
			b.Fatalf("encodeDocumentMetadata(): %v", err)
		}
	}
}
