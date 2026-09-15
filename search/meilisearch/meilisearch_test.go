package meilisearch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/search"
)

const (
	testTaskResponse   = `{"taskUid":1,"indexUid":"idx","status":"enqueued"}`
	testErrorResponse  = `{"message":"boom"}`
	testSearchResponse = `{"hits":[{"id":"doc-1","content":"hello","metadata":{"k":"v"},"_rankingScore":0.9}],"totalHits":1,"processingTimeMs":1,"query":"hello"}`
)

type capturedRequest struct {
	method string
	path   string
	auth   string
	body   []byte
}

func serveJSON(t *testing.T, status int, body string, capt *capturedRequest) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if capt != nil {
			capt.method = r.Method
			capt.path = r.URL.Path
			capt.auth = r.Header.Get("Authorization")
			b, err := io.ReadAll(r.Body)
			if err == nil {
				capt.body = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func openTest(t *testing.T, srv *httptest.Server, key string) search.Search {
	t.Helper()

	s, err := Open(search.Options{Host: srv.URL, APIKey: key})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	return s
}

func TestOpen_missingHost_returnsErrMissingHost(t *testing.T) {
	t.Parallel()

	if _, err := Open(search.Options{}); !errors.Is(err, ErrMissingHost) {
		t.Fatalf("Open err = %v, want ErrMissingHost", err)
	}
}

func TestOpen_invalidOptions_wrapsErrInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := Open(search.Options{Host: "localhost:7700"})
	if !errors.Is(err, search.ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want wrap of ErrInvalidOptions", err)
	}
}

func TestOpen_apiKey_setsBearerHeader(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(serveJSON(t, 202, testTaskResponse, &capt))
	defer srv.Close()

	s := openTest(t, srv, "secret")
	defer func() { _ = s.Close() }()

	if err := s.Index(t.Context(), search.Document{ID: "d1", Index: "idx", Content: "hi"}); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	if capt.auth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want %q", capt.auth, "Bearer secret")
	}
}

func TestOpen_anonymous_sendsNoAuthHeader(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(serveJSON(t, 202, testTaskResponse, &capt))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	if err := s.Index(t.Context(), search.Document{ID: "d1", Index: "idx", Content: "hi"}); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	if capt.auth != "" {
		t.Fatalf("Authorization = %q, want empty", capt.auth)
	}
}

func TestIndex_sendsDocumentStructure(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(serveJSON(t, 202, testTaskResponse, &capt))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	meta := map[string]any{"k": "v"}
	if err := s.Index(t.Context(), search.Document{ID: "d1", Index: "idx", Content: "hello", Metadata: meta}); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	if capt.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", capt.method)
	}

	if capt.path != "/indexes/idx/documents" {
		t.Fatalf("path = %q, want /indexes/idx/documents", capt.path)
	}

	var docs []map[string]any
	if err := json.Unmarshal(capt.body, &docs); err != nil {
		t.Fatalf("add body unmarshal err = %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}

	if docs[0]["id"] != "d1" || docs[0]["content"] != "hello" {
		t.Fatalf("doc = %v, want id d1 content hello", docs[0])
	}

	gotMeta, ok := docs[0]["metadata"].(map[string]any)
	if !ok || gotMeta["k"] != "v" {
		t.Fatalf("metadata = %v, want map[k:v]", docs[0]["metadata"])
	}

	if len(meta) != 1 || meta["k"] != "v" {
		t.Fatalf("caller metadata mutated: %v", meta)
	}
}

func TestIndex_error_returnsErrorWithoutPhantomTrack(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 500, testErrorResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	if err := s.Index(t.Context(), search.Document{ID: "d1", Index: "idx", Content: "hi"}); err == nil {
		t.Fatal("Index err = nil, want error")
	}

	err := s.Delete(t.Context(), "d1")
	var nf *search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Delete err = %v, want NotFoundError (no phantom track)", err)
	}
}

func TestDelete_untracked_returnsNotFoundWithHint(t *testing.T) {
	t.Parallel()

	s := &meilisearchClient{client: nil, idIndexes: newIDIndexTracker(maxIDIndexes)}
	defer func() { _ = s.Close() }()

	err := s.Delete(t.Context(), "ghost")

	var nf *search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Delete err type = %T, want *NotFoundError", err)
	}

	if nf.ID != "ghost" {
		t.Fatalf("NotFoundError.ID = %q, want ghost", nf.ID)
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("Delete err = %v, want wrap of ErrNotFound", err)
	}

	for _, hint := range []string{"re-Index before Delete", "lost on restart", "unsafe with multiple instances"} {
		if !strings.Contains(err.Error(), hint) {
			t.Fatalf("Delete err %q missing hint %q", err.Error(), hint)
		}
	}
}

func TestDelete_fanOut_deletesFromTwoIndexes(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	var mu sync.Mutex
	paths := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			serveJSON(t, 202, testTaskResponse, nil)(w, r)

			return
		}
		if r.Method == http.MethodDelete {
			calls.Add(1)
			mu.Lock()
			paths[r.URL.Path] = true
			mu.Unlock()
			serveJSON(t, 202, testTaskResponse, nil)(w, r)

			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	if err := s.Index(ctx, search.Document{ID: "d1", Index: "a", Content: "x"}); err != nil {
		t.Fatalf("Index a err = %v", err)
	}

	if err := s.Index(ctx, search.Document{ID: "d1", Index: "b", Content: "x"}); err != nil {
		t.Fatalf("Index b err = %v", err)
	}

	if err := s.Delete(ctx, "d1"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("DELETE calls = %d, want 2", got)
	}

	for _, want := range []string{"/indexes/a/documents/d1", "/indexes/b/documents/d1"} {
		mu.Lock()
		ok := paths[want]
		mu.Unlock()
		if !ok {
			t.Fatalf("missing DELETE %s, got %v", want, paths)
		}
	}

	if err := s.Delete(ctx, "d1"); !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("second Delete err = %v, want ErrNotFound", err)
	}
}

func TestDelete_perIndexError_returnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			serveJSON(t, 202, testTaskResponse, nil)(w, r)

			return
		}
		serveJSON(t, 500, testErrorResponse, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	if err := s.Index(ctx, search.Document{ID: "d1", Index: "idx", Content: "x"}); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	err := s.Delete(ctx, "d1")
	if err == nil || !strings.Contains(err.Error(), "delete") {
		t.Fatalf("Delete err = %v, want delete error", err)
	}
}

func TestDelete_afterSearch_seededIndexSucceeds(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search"):
			serveJSON(t, 200, testSearchResponse, nil)(w, r)
		case r.Method == http.MethodDelete:
			if r.URL.Path != "/indexes/idx/documents/doc-1" {
				t.Errorf("DELETE path = %q, want /indexes/idx/documents/doc-1", r.URL.Path)
			}
			serveJSON(t, 202, testTaskResponse, nil)(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	res, err := s.Search(ctx, "hello", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if len(res.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(res.Hits))
	}

	if err := s.Delete(ctx, "doc-1"); err != nil {
		t.Fatalf("Delete err = %v, want nil (search seeded track)", err)
	}
}

func TestSearch_emptyQuery_shortCircuitsWithoutHTTP(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	res, err := s.Search(t.Context(), "", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if hits.Load() != 0 {
		t.Fatalf("server hits = %d, want 0", hits.Load())
	}

	if len(res.Hits) != 0 || res.Total != 0 {
		t.Fatalf("result = %+v, want empty", res)
	}
}

func TestSearch_withoutIndex_returnsErrIndexRequired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 200, testSearchResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	for name, filters := range map[string]map[string]string{
		"nil":         nil,
		"empty":       {},
		"empty value": {"index": ""},
		"other key":   {"other": "idx"},
	} {
		_, err := s.Search(t.Context(), "q", search.QueryOptions{Filters: filters})
		if !errors.Is(err, ErrIndexRequired) {
			t.Fatalf("%s: Search err = %v, want ErrIndexRequired", name, err)
		}
	}
}

func TestSearch_appliesLimitOffsetAndRankingScore(t *testing.T) {
	t.Parallel()

	var capt capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capt.method = r.Method
		capt.path = r.URL.Path
		capt.body = b

		serveJSON(t, 200, testSearchResponse, nil)(w, r)
	}))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	res, err := s.Search(t.Context(), "hello", search.QueryOptions{Limit: 5, Offset: 3, Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if capt.method != http.MethodPost || capt.path != "/indexes/idx/search" {
		t.Fatalf("request = %s %s, want POST /indexes/idx/search", capt.method, capt.path)
	}

	var body map[string]any
	if err := json.Unmarshal(capt.body, &body); err != nil {
		t.Fatalf("search body unmarshal err = %v", err)
	}

	if body["q"] != "hello" {
		t.Fatalf("q = %v, want hello", body["q"])
	}

	if body["limit"] != float64(5) || body["offset"] != float64(3) {
		t.Fatalf("limit/offset = %v/%v, want 5/3", body["limit"], body["offset"])
	}

	if body["showRankingScore"] != true {
		t.Fatalf("showRankingScore = %v, want true", body["showRankingScore"])
	}

	if len(res.Hits) != 1 || res.Hits[0].ID != "doc-1" {
		t.Fatalf("hits = %+v, want doc-1", res.Hits)
	}

	if res.Hits[0].Score != 0.9 {
		t.Fatalf("score = %v, want 0.9", res.Hits[0].Score)
	}

	if res.Hits[0].Metadata["k"] != "v" {
		t.Fatalf("metadata = %v, want map[k:v]", res.Hits[0].Metadata)
	}

	if res.Total != 1 {
		t.Fatalf("total = %d, want 1", res.Total)
	}
}

func TestSearch_defaultLimitAndClampedOffset(t *testing.T) {
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

	res, err := s.Search(t.Context(), "q", search.QueryOptions{Offset: -5, Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(capt.body, &body); err != nil {
		t.Fatalf("body unmarshal err = %v", err)
	}

	if body["limit"] != float64(search.DefaultLimit) {
		t.Fatalf("limit = %v, want %d", body["limit"], search.DefaultLimit)
	}

	if body["offset"] != nil && body["offset"] != float64(0) {
		t.Fatalf("offset = %v, want absent or 0", body["offset"])
	}

	if len(res.Hits) != 0 || res.Total != 0 {
		t.Fatalf("result = %+v, want empty", res)
	}
}

func TestSearch_malformedHitFields_skipsBadFieldsKeepsHit(t *testing.T) {
	t.Parallel()

	payload := `{"hits":[{"id":42,"content":"x","metadata":["nope"],"_rankingScore":"high"}],"totalHits":1,"processingTimeMs":1,"query":"q"}`
	srv := httptest.NewServer(serveJSON(t, 200, payload, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	res, err := s.Search(t.Context(), "q", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if len(res.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(res.Hits))
	}

	if res.Hits[0].ID != "" {
		t.Fatalf("ID = %q, want empty (numeric id skipped)", res.Hits[0].ID)
	}

	if res.Hits[0].Score != 0 {
		t.Fatalf("Score = %v, want 0 (string score skipped)", res.Hits[0].Score)
	}

	if res.Hits[0].Metadata != nil {
		t.Fatalf("Metadata = %v, want nil (array metadata skipped)", res.Hits[0].Metadata)
	}
}

func TestSearch_httpError_returnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(serveJSON(t, 500, testErrorResponse, nil))
	defer srv.Close()

	s := openTest(t, srv, "")
	defer func() { _ = s.Close() }()

	_, err := s.Search(t.Context(), "q", search.QueryOptions{Filters: map[string]string{"index": "idx"}})
	if err == nil || !strings.Contains(err.Error(), "search") {
		t.Fatalf("Search err = %v, want search error", err)
	}
}

func TestToHits_missingKeys_yieldsZeroHit(t *testing.T) {
	t.Parallel()

	hits := toHits(nil)
	if len(hits) != 0 {
		t.Fatalf("toHits(nil) = %v, want empty", hits)
	}
}

func TestTracker_addGetCloneDedupMoveBackEvict(t *testing.T) {
	t.Parallel()

	tr := newIDIndexTracker(2)
	tr.add("a", []string{"i1"})
	tr.add("b", []string{"i2"})

	got, ok := tr.get("a")
	if !ok || len(got) != 1 || got[0] != "i1" {
		t.Fatalf("get(a) = %v,%v, want [i1],true", got, ok)
	}

	got[0] = "mutated"
	again, _ := tr.get("a")
	if again[0] != "i1" {
		t.Fatalf("clone broken: get(a) = %v after mutate", again)
	}

	tr.add("a", recordIndex([]string{"i1"}, "i1"))
	if got, _ := tr.get("a"); len(got) != 1 {
		t.Fatalf("dedup failed: get(a) = %v, want [i1]", got)
	}

	tr.add("a", recordIndex([]string{"i1"}, "i3"))
	if got, _ := tr.get("a"); len(got) != 2 {
		t.Fatalf("record failed: get(a) = %v, want [i1 i3]", got)
	}

	tr.add("c", []string{"i4"})
	if _, ok := tr.get("b"); ok {
		t.Fatal("evict failed: b still present after cap overflow")
	}

	if _, ok := tr.get("a"); !ok {
		t.Fatal("move-back failed: a evicted instead of b")
	}

	if _, ok := tr.get("c"); !ok {
		t.Fatal("c missing after add")
	}

	if tr.len() != 2 {
		t.Fatalf("len = %d, want 2", tr.len())
	}
}

func TestTracker_evictSkipsCorruptOrderEntry(t *testing.T) {
	t.Parallel()

	tr := newIDIndexTracker(1)
	tr.order.PushFront(123)
	tr.add("a", []string{"i1"})
	tr.add("b", []string{"i2"})

	if _, ok := tr.get("a"); ok {
		t.Fatal("a should have been evicted")
	}

	if _, ok := tr.get("b"); !ok {
		t.Fatal("b missing after evict")
	}
}

func TestTracker_evictWithDrainedOrder_breaksCleanly(t *testing.T) {
	t.Parallel()

	tr := newIDIndexTracker(1)
	tr.entries["x"] = []string{"i1"}
	tr.entries["y"] = []string{"i2"}
	tr.order.PushFront(123)
	tr.add("z", []string{"i3"})

	if tr.len() != 2 {
		t.Fatalf("len = %d, want 2", tr.len())
	}

	if _, ok := tr.get("z"); ok {
		t.Fatal("z should have been evicted as oldest after corrupt skip")
	}
}

func TestTracker_delete_missingAndOrphanEntry(t *testing.T) {
	t.Parallel()

	tr := newIDIndexTracker(10)
	tr.delete("ghost")

	tr.add("a", []string{"i1"})
	delete(tr.elements, "a")
	tr.delete("a")

	if _, ok := tr.get("a"); ok {
		t.Fatal("a should be deleted")
	}

	tr.add("b", []string{"i1"})
	tr.delete("b")

	if tr.len() != 0 {
		t.Fatalf("len = %d, want 0", tr.len())
	}
}

func TestRecordIndex_table(t *testing.T) {
	t.Parallel()

	if got := recordIndex(nil, "x"); len(got) != 1 || got[0] != "x" {
		t.Fatalf("recordIndex(nil,x) = %v", got)
	}

	if got := recordIndex([]string{"x"}, "x"); len(got) != 1 {
		t.Fatalf("recordIndex dup = %v, want [x]", got)
	}

	if got := recordIndex([]string{"x"}, "y"); len(got) != 2 {
		t.Fatalf("recordIndex append = %v, want [x y]", got)
	}
}

func TestResolveIndex_table(t *testing.T) {
	t.Parallel()

	got, err := resolveIndex(map[string]string{"index": "idx"})
	if err != nil || got != "idx" {
		t.Fatalf("resolveIndex = %q,%v, want idx,nil", got, err)
	}
}

func TestClose_returnsNil(t *testing.T) {
	t.Parallel()

	s := &meilisearchClient{client: nil, idIndexes: newIDIndexTracker(1)}
	if err := s.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}
}
