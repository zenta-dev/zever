package meilisearch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meilisearch/meilisearch-go"

	"github.com/zenta-dev/zever/core/search"
)

// newBenchSearch opens a client against an in-process stub Meilisearch server
// that answers document writes with a task and searches with a fixed hit.
func newBenchSearch(b *testing.B) search.Search {
	b.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/search") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testSearchResponse))

			return
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(testTaskResponse))
	}))
	b.Cleanup(srv.Close)

	s, err := New(search.Options{Host: srv.URL, AllowInsecure: true})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// BenchmarkIndex measures the full single-document write round trip against
// the stub server, including the id-index tracking update.
func BenchmarkIndex(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()
	doc := search.Document{ID: "doc-1", Index: "idx", Content: "hello", Metadata: map[string]any{"k": "v"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Index(ctx, doc); err != nil {
			b.Fatalf("Index(): %v", err)
		}
	}
}

// BenchmarkSearch measures the ranked search round trip against the stub
// server, including hit decoding and id-index tracking.
func BenchmarkSearch(b *testing.B) {
	s := newBenchSearch(b)
	ctx := b.Context()
	opts := search.QueryOptions{Filters: map[string]string{"index": "idx"}, Limit: 10}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Search(ctx, "hello", opts); err != nil {
			b.Fatalf("Search(): %v", err)
		}
	}
}

// BenchmarkToHits measures decoding raw Meilisearch hits into search.Hit
// values (the pure post-processing path).
func BenchmarkToHits(b *testing.B) {
	raw := meilisearch.Hits{
		{
			"id":            json.RawMessage(`"doc-1"`),
			"content":       json.RawMessage(`"hello"`),
			"metadata":      json.RawMessage(`{"k":"v"}`),
			"_rankingScore": json.RawMessage(`0.9`),
		},
		{
			"id":            json.RawMessage(`"doc-2"`),
			"content":       json.RawMessage(`"world"`),
			"metadata":      json.RawMessage(`{"k":"w"}`),
			"_rankingScore": json.RawMessage(`0.5`),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if hits := toHits(raw); len(hits) == 0 {
			b.Fatal("toHits() returned no hits")
		}
	}
}
