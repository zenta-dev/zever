package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// benchStore is a preloaded in-memory store for retrieve benchmarks.
func benchEngine(b *testing.B) *Engine {
	b.Helper()

	f := &fakeAI{gen: ai.Generation{Content: "answer"}}
	s := &memStore{vecs: []vectorstore.Vector{
		{ID: "a#0", Metadata: map[string]any{"content": "first context"}},
		{ID: "a#1", Metadata: map[string]any{"content": "second context"}},
		{ID: "a#2", Metadata: map[string]any{"content": "third context"}},
	}}
	e, err := New(f, s, Options{Model: "m", ChunkSize: 64, ChunkOverlap: 8})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	return e
}

// BenchmarkNew measures Engine construction.
func BenchmarkNew(b *testing.B) {
	f := &fakeAI{}
	s := &memStore{}
	opts := Options{Model: "m"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := New(f, s, opts); err != nil {
			b.Fatalf("New() error = %v", err)
		}
	}
}

// BenchmarkChunkText measures the chunking hot path.
func BenchmarkChunkText(b *testing.B) {
	text := strings.Repeat("abcdefghij", 100)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = chunkText(text, 64, 8)
	}
}

// BenchmarkDocumentValidate measures document validation.
func BenchmarkDocumentValidate(b *testing.B) {
	d := Document{ID: "d1", Content: "hello"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := d.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

// BenchmarkIngest measures chunk + batch-embed + upsert.
func BenchmarkIngest(b *testing.B) {
	text := strings.Repeat("abcdefghij", 50)
	docs := []Document{{ID: "d1", Content: text}}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		f := &fakeAI{}
		s := &memStore{}
		fresh, err := New(f, s, Options{Model: "m", ChunkSize: 64, ChunkOverlap: 8})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}
		if err := fresh.Ingest(ctx, docs); err != nil {
			b.Fatalf("Ingest() error = %v", err)
		}
	}
}

// BenchmarkRetrieve measures query embed plus store query.
func BenchmarkRetrieve(b *testing.B) {
	e := benchEngine(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := e.Retrieve(ctx, "what?", 2); err != nil {
			b.Fatalf("Retrieve() error = %v", err)
		}
	}
}

// BenchmarkAnswer measures retrieve plus grounded generation.
func BenchmarkAnswer(b *testing.B) {
	e := benchEngine(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := e.Answer(ctx, "what?", 2); err != nil {
			b.Fatalf("Answer() error = %v", err)
		}
	}
}

// BenchmarkWithDefaults measures option resolution.
func BenchmarkWithDefaults(b *testing.B) {
	opts := Options{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = opts.withDefaults()
	}
}
