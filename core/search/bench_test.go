package search

import (
	"testing"
)

// BenchmarkOptionsValidate measures Options validation for a search backend.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Host: "https://example.com"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkDocumentValidate(b *testing.B) {
	doc := Document{
		ID:      "doc-1",
		Index:   "main",
		Content: "hello world",
		Metadata: map[string]any{
			"author": "alice",
			"tags":   []string{"go", "search"},
			"score":  float64(1.5),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := doc.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
