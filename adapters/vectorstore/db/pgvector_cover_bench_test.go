package db

import (
	"testing"
)

// BenchmarkDecodeHitMeta measures single-hit metadata decode.
func BenchmarkDecodeHitMeta(b *testing.B) {
	raw := []byte(`{"kind":"bench","n":1}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := decodeHitMeta(raw); err != nil {
			b.Fatalf("decodeHitMeta(): %v", err)
		}
	}
}

// BenchmarkEncodeMetadata measures metadata encode.
func BenchmarkEncodeMetadata(b *testing.B) {
	m := map[string]any{"kind": "bench", "n": 1}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := encodeMetadata(m); err != nil {
			b.Fatalf("encodeMetadata(): %v", err)
		}
	}
}

// BenchmarkTableDDL measures postgres DDL rendering.
func BenchmarkTableDDL(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = tableDDL(1536)
	}
}

// BenchmarkParseVectorType measures vector type parsing.
func BenchmarkParseVectorType(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := parseVectorType("vector(1536)"); err != nil {
			b.Fatalf("parseVectorType(): %v", err)
		}
	}
}
