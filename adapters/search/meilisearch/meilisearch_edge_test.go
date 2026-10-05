package meilisearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/meilisearch/meilisearch-go"

	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/shared/lrucache"
)

func TestEdgeNew_whitespaceHost(t *testing.T) {
	t.Parallel()

	if _, err := New(search.Options{Host: "   "}); !errors.Is(err, search.ErrInvalidOptions) {
		t.Fatalf("New(whitespace host) err = %v, want ErrInvalidOptions", err)
	}
}

func TestEdgeIndex_invalidDoc(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 202, testTaskResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	bad := search.Document{
		ID:       "bad",
		Index:    "idx",
		Content:  "probe",
		Metadata: map[string]any{"ch": make(chan int)},
	}

	if err := s.Index(t.Context(), bad); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("Index() err = %v, want ErrInvalidMetadata", err)
	}
}

func TestEdgeIndexBatch_invalidDocAtIndex(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 202, testTaskResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	docs := []search.Document{
		{ID: "ok", Index: "idx", Content: "fine"},
		{ID: "bad", Index: "idx", Content: "boom", Metadata: map[string]any{"ch": make(chan int)}},
	}

	err := s.IndexBatch(t.Context(), docs)
	if err == nil {
		t.Fatal("IndexBatch() err = nil, want error")
	}

	if !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("IndexBatch() err = %v, want ErrInvalidMetadata", err)
	}

	if !strings.Contains(err.Error(), "index 1") {
		t.Fatalf("IndexBatch() err = %v, want index 1 in message", err)
	}
}

func TestEdgeDelete_emptyID(t *testing.T) {
	t.Parallel()

	s := &meilisearchClient{client: nil, idIndexes: lrucache.New[string, []string](maxIDIndexes)}
	defer func() { _ = s.Close() }()

	err := s.Delete(t.Context(), "")
	if err == nil {
		t.Fatal("Delete(\"\") err = nil, want NotFoundError")
	}

	var nf search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Delete(\"\") err = %v, want NotFoundError", err)
	}

	if nf.ID != "" {
		t.Fatalf("NotFoundError.ID = %q, want empty", nf.ID)
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("Delete(\"\") err = %v, want ErrNotFound", err)
	}
}

func TestEdgeSearch_whitespaceQuery(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		serveJSON(t, http.StatusOK, `{"hits":[],"totalHits":0,"processingTimeMs":1,"query":"   "}`, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	res, err := s.Search(t.Context(), "   ", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1 (whitespace query goes to HTTP)", hits.Load())
	}

	if len(res.Hits) != 0 || res.Total != 0 {
		t.Fatalf("Search(\"   \") = %+v, want empty", res)
	}
}

func TestEdgeSearch_negativeLimit(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capt.body = b
		serveJSON(t, 200, `{"hits":[],"totalHits":0,"processingTimeMs":1,"query":"q"}`, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	_, err := s.Search(t.Context(), "q", search.QueryOptions{Limit: -1, Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(capt.body, &body); err != nil {
		t.Fatalf("body unmarshal err = %v", err)
	}

	if body["limit"] != float64(search.DefaultLimit) {
		t.Fatalf("limit = %v, want %d", body["limit"], search.DefaultLimit)
	}
}

func TestEdgeSearch_negativeOffset(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capt.body = b
		serveJSON(t, 200, `{"hits":[],"totalHits":0,"processingTimeMs":1,"query":"q"}`, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	_, err := s.Search(t.Context(), "q", search.QueryOptions{Offset: -5, Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(capt.body, &body); err != nil {
		t.Fatalf("body unmarshal err = %v", err)
	}

	if body["offset"] != nil && body["offset"] != float64(0) {
		t.Fatalf("offset = %v, want absent or 0", body["offset"])
	}
}

func TestEdgeSearch_malformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 200, `{"hits":`, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	_, err := s.Search(t.Context(), "q", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err == nil {
		t.Fatal("Search() err = nil, want error")
	}
}

func TestEdgeSearch_emptyHits(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 200, `{"hits":[],"totalHits":0,"processingTimeMs":1,"query":"q"}`, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	res, err := s.Search(t.Context(), "q", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if len(res.Hits) != 0 || res.Total != 0 {
		t.Fatalf("Search() = %+v, want empty", res)
	}
}

func TestEdgeSearch_contextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 200, testSearchResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.Search(ctx, "q", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err == nil {
		t.Fatal("Search(cancelled ctx) err = nil, want error")
	}
}

func TestEdgeToHits_emptyRaw(t *testing.T) {
	t.Parallel()

	hits := toHits(meilisearch.Hits{})
	if len(hits) != 0 {
		t.Fatalf("toHits(empty) = %v, want empty", hits)
	}
}

func TestEdgeCloneIndexes_nil(t *testing.T) {
	t.Parallel()

	if got := cloneIndexes(nil); got != nil {
		t.Fatalf("cloneIndexes(nil) = %v, want nil (no allocation)", got)
	}
}

func TestEdgeClient_concurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/search") {
			serveJSON(t, 200, testSearchResponse, nil)(w, r)

			return
		}

		serveJSON(t, 202, testTaskResponse, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	ctx := t.Context()

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			doc := search.Document{
				ID:      "conc-" + strings.Repeat("x", i),
				Index:   "conc",
				Content: "concurrent probe document",
			}

			if err := s.Index(ctx, doc); err != nil {
				t.Errorf("Index() err = %v", err)

				return
			}

			if _, err := s.Search(ctx, "concurrent", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "conc"}}); err != nil {
				t.Errorf("Search() err = %v", err)

				return
			}
		}(i)
	}

	wg.Wait()
}

func TestEdgeClose_nilClient(t *testing.T) {
	t.Parallel()

	s := &meilisearchClient{client: nil, idIndexes: lrucache.New[string, []string](1)}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() err = %v, want nil", err)
	}
}
