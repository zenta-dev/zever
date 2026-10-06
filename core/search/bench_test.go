package search

import (
	"testing"
)

// benchSearchAdapter registers a stub search once and returns its adapter so
// Open can be measured without paying the one-shot registration cost.
func benchSearchAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Search, error) { return &stubSearch{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchSearchAdapter(b)
	opts := Options{Host: "https://example.com"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, opts); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchSearchAdapter(b)
	opts := Options{Host: "https://example.com"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, opts); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

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
