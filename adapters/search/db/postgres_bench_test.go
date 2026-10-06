package db

import (
	"path/filepath"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
)

// newBenchSearch opens a file-backed sqlite search adapter. A file (not
// ":memory:") keeps each benchmark isolated from process-global shared cache.
func newBenchSearch(b *testing.B) search.Search {
	b.Helper()

	s, err := New(Options{Options: coredb.Options{Path: filepath.Join(b.TempDir(), "search.db")}})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// benchDoc builds one document for the bench index.
func benchDoc(id string) search.Document {
	return search.Document{
		ID:       id,
		Index:    "idx",
		Content:  "the quick brown fox jumps over the lazy dog",
		Metadata: map[string]any{"kind": "bench"},
	}
}

// BenchmarkIndex measures the single-document upsert: metadata encode plus an
// INSERT ... ON CONFLICT write and its FTS5 trigger.
func BenchmarkIndex(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()
	doc := benchDoc("bench-1")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Index(ctx, doc); err != nil {
			b.Fatalf("Index(): %v", err)
		}
	}
}

// BenchmarkSearch measures the ranked FTS5 query plus its COUNT(*) twin over
// a fixed corpus.
func BenchmarkSearch(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()

	for i := 0; i < 128; i++ {
		if err := s.Index(ctx, benchDoc("doc-"+string(rune('a'+i%26))+string(rune('a'+i/26)))); err != nil {
			b.Fatalf("seed Index(): %v", err)
		}
	}

	opts := search.QueryOptions{Filters: map[string]string{"index": "idx"}, Limit: 10}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Search(ctx, "quick fox", opts); err != nil {
			b.Fatalf("Search(): %v", err)
		}
	}
}

// BenchmarkIndexBatch measures a small bulk index: one upsert per document.
func BenchmarkIndexBatch(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()

	docs := make([]search.Document, 8)
	for i := range docs {
		docs[i] = benchDoc("batch-" + string(rune('a'+i)))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.IndexBatch(ctx, docs); err != nil {
			b.Fatalf("IndexBatch(): %v", err)
		}
	}
}

// BenchmarkDelete measures the single-document delete: PK lookup plus the
// RowsAffected check.
func BenchmarkDelete(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()

	doc := benchDoc("bench-del")

	if err := s.Index(ctx, doc); err != nil {
		b.Fatalf("seed Index(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Delete(ctx, doc.ID); err != nil {
			b.Fatalf("Delete(): %v", err)
		}

		if err := s.Index(ctx, doc); err != nil {
			b.Fatalf("reseed Index(): %v", err)
		}
	}
}
