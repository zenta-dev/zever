package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/search"
)

func newMemoryStore(t *testing.T) *Store {
	t.Helper()

	sr, err := New(search.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	s, ok := sr.(*Store)
	if !ok {
		t.Fatalf("New() returned %T, want *Store", sr)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func mustIndex(t *testing.T, s *Store, doc search.Document) {
	t.Helper()

	if err := s.Index(context.Background(), doc); err != nil {
		t.Fatalf("Index(%q): %v", doc.ID, err)
	}
}

func mustSearch(t *testing.T, s *Store, query string, opts search.QueryOptions) search.Result {
	t.Helper()

	res, err := s.Search(context.Background(), query, opts)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}

	return res
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = s.Close() }()

	ctx := context.Background()

	if indexErr := s.Index(ctx, search.Document{ID: "d1", Index: "main", Content: "hello world"}); indexErr != nil {
		t.Fatal(indexErr)
	}

	res, err := s.Search(ctx, "hello", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "d1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{Host: "localhost:7700"})
	if !errors.Is(err, search.ErrInvalidOptions) {
		t.Fatalf("New err = %v, want ErrInvalidOptions", err)
	}

	if s != nil {
		t.Fatal("expected nil search on invalid options")
	}
}

func TestNewBadDirDSN(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: filepath.Join(t.TempDir(), "no-such-dir", "search.db")})
	if err == nil {
		t.Fatal("expected error for bad-dir DSN, got nil")
	}

	if s != nil {
		t.Fatal("expected nil search on bad-dir DSN")
	}
}

func TestNewInvalidDSN(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: "bad;dsn"})
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}

	if s != nil {
		t.Fatal("expected nil store for invalid DSN")
	}
}

func TestNewOpenError(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: "file:x?%zz"})
	if err == nil {
		t.Fatal("expected open error for malformed DSN, got nil")
	}

	if !strings.Contains(err.Error(), "sqlite: open") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: open")
	}

	if s != nil {
		t.Fatal("expected nil store for malformed DSN")
	}
}

func TestRoundtrip(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{
		ID:       "doc-1",
		Index:    "main",
		Content:  "the quick brown fox jumps",
		Metadata: map[string]any{"topic": "animals"},
	})

	res, err := s.Search(ctx, "quick brown", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "main"}})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 1 {
		t.Fatalf("Total = %d, want 1", res.Total)
	}

	if len(res.Hits) != 1 {
		t.Fatalf("Hits = %+v, want one hit", res.Hits)
	}

	hit := res.Hits[0]
	if hit.ID != "doc-1" {
		t.Fatalf("hit ID = %q, want doc-1", hit.ID)
	}

	if hit.Score <= 0 {
		t.Fatalf("hit Score = %v, want > 0", hit.Score)
	}

	if hit.Metadata["topic"] != "animals" {
		t.Fatalf("hit metadata topic = %v, want animals", hit.Metadata["topic"])
	}

	if hit.Metadata["content"] != "the quick brown fox jumps" {
		t.Fatalf("hit metadata content = %v, want injected content", hit.Metadata["content"])
	}
}

func TestExactTokenNoStemming(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "running fast"})

	// The default FTS5 tokenizer does not stem: "run" must not match "running".
	res := mustSearch(t, s, "run", search.QueryOptions{Limit: 10})
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("stemmed query matched: %+v", res)
	}

	res = mustSearch(t, s, "running", search.QueryOptions{Limit: 10})
	if res.Total != 1 {
		t.Fatalf("exact token Total = %d, want 1", res.Total)
	}

	// Tokenizer still folds case.
	res, err := s.Search(ctx, "RUNNING", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 1 {
		t.Fatalf("uppercase Total = %d, want 1", res.Total)
	}
}

func TestRanking(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	mustIndex(t, s, search.Document{ID: "once", Index: "main", Content: "apple banana cherry"})
	mustIndex(t, s, search.Document{ID: "thrice", Index: "main", Content: "apple apple apple"})

	res := mustSearch(t, s, "apple", search.QueryOptions{Limit: 10})
	if len(res.Hits) != 2 {
		t.Fatalf("Hits = %+v, want 2", res.Hits)
	}

	if res.Hits[0].ID != "thrice" {
		t.Fatalf("first hit = %q, want thrice", res.Hits[0].ID)
	}

	if res.Hits[0].Score <= res.Hits[1].Score {
		t.Fatalf("scores not ranked: %v <= %v", res.Hits[0].Score, res.Hits[1].Score)
	}
}

func TestIndexFilterIsolation(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	mustIndex(t, s, search.Document{ID: "a1", Index: "a", Content: "shared token alpha"})
	mustIndex(t, s, search.Document{ID: "b1", Index: "b", Content: "shared token beta"})

	res := mustSearch(t, s, "shared", search.QueryOptions{Limit: 10})
	if res.Total != 2 {
		t.Fatalf("unfiltered Total = %d, want 2", res.Total)
	}

	res = mustSearch(t, s, "shared", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "a"}})
	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "a1" {
		t.Fatalf("filtered result = %+v, want one hit a1", res.Hits)
	}

	res = mustSearch(t, s, "shared", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "missing"}})
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("missing-index result = %+v, want empty", res)
	}
}

func TestEmptyQuery(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "hello world"})

	for _, q := range []string{"", "   ", `"`} {
		res, err := s.Search(context.Background(), q, search.QueryOptions{Limit: 10})
		if err != nil {
			t.Fatalf("Search(%q) err = %v", q, err)
		}

		if res.Total != 0 || len(res.Hits) != 0 {
			t.Fatalf("Search(%q) = %+v, want empty", q, res)
		}
	}
}

func TestLimitOffset(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	for _, id := range []string{"i1", "i2", "i3"} {
		mustIndex(t, s, search.Document{ID: id, Index: "main", Content: "item " + id})
	}

	res := mustSearch(t, s, "item", search.QueryOptions{})
	if len(res.Hits) != search.DefaultLimit && len(res.Hits) != 3 {
		t.Fatalf("default limit Hits = %d, want 3", len(res.Hits))
	}

	if res.Total != 3 {
		t.Fatalf("default limit Total = %d, want 3", res.Total)
	}

	res = mustSearch(t, s, "item", search.QueryOptions{Limit: -5})
	if len(res.Hits) != 3 {
		t.Fatalf("negative limit Hits = %d, want 3", len(res.Hits))
	}

	res = mustSearch(t, s, "item", search.QueryOptions{Limit: 2})
	if len(res.Hits) != 2 || res.Total != 3 {
		t.Fatalf("limit 2 = %+v, want 2 hits total 3", res)
	}

	first := res.Hits[0].ID

	res = mustSearch(t, s, "item", search.QueryOptions{Limit: 2, Offset: 1})
	if len(res.Hits) != 2 || res.Total != 3 {
		t.Fatalf("offset 1 = %+v, want 2 hits total 3", res)
	}

	if res.Hits[0].ID == first {
		t.Fatalf("offset 1 first hit = %q, want it skipped", res.Hits[0].ID)
	}

	res = mustSearch(t, s, "item", search.QueryOptions{Limit: 2, Offset: -3})
	if len(res.Hits) != 2 || res.Hits[0].ID != first {
		t.Fatalf("negative offset = %+v, want clamp to 0", res)
	}

	res = mustSearch(t, s, "item", search.QueryOptions{Limit: 2, Offset: 10})
	if len(res.Hits) != 0 || res.Total != 3 {
		t.Fatalf("offset past end = %+v, want empty hits total 3", res)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "goodbye world"})

	if err := s.Delete(ctx, "d1"); err != nil {
		t.Fatal(err)
	}

	res := mustSearch(t, s, "goodbye", search.QueryOptions{Limit: 10})
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("after delete = %+v, want empty", res)
	}

	err := s.Delete(ctx, "d1")
	if err == nil {
		t.Fatal("expected NotFound on second delete, got nil")
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	var nf *search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %T is not *NotFoundError", err)
	}

	if nf.ID != "d1" {
		t.Fatalf("carried ID = %q, want d1", nf.ID)
	}

	if !strings.Contains(err.Error(), "sqlite: delete") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: delete")
	}
}

func TestDeleteMissing(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	err := s.Delete(context.Background(), "nope")
	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteCrossIndex(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	mustIndex(t, s, search.Document{ID: "dup", Index: "a", Content: "duplicated entry"})
	mustIndex(t, s, search.Document{ID: "dup", Index: "b", Content: "duplicated entry"})

	if err := s.Delete(context.Background(), "dup"); err != nil {
		t.Fatal(err)
	}

	res := mustSearch(t, s, "duplicated", search.QueryOptions{Limit: 10})
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("after cross-index delete = %+v, want empty", res)
	}
}

func TestMetadataRoundtripUnmutated(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	meta := map[string]any{"k": "v", "n": float64(1)}

	mustIndex(t, s, search.Document{ID: "m1", Index: "main", Content: "hello world meta", Metadata: meta})

	if _, ok := meta["content"]; ok {
		t.Fatal("caller metadata mutated with injected content")
	}

	if len(meta) != 2 {
		t.Fatalf("caller metadata = %v, want untouched", meta)
	}

	res := mustSearch(t, s, "hello", search.QueryOptions{Limit: 10})
	if len(res.Hits) != 1 {
		t.Fatalf("Hits = %+v, want 1", res.Hits)
	}

	if res.Hits[0].Metadata["k"] != "v" || res.Hits[0].Metadata["n"] != float64(1) {
		t.Fatalf("metadata = %v, want roundtripped", res.Hits[0].Metadata)
	}

	if res.Hits[0].Metadata["content"] != "hello world meta" {
		t.Fatalf("metadata content = %v, want injected", res.Hits[0].Metadata["content"])
	}
}

func TestMetadataContentKeyOverwritten(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	meta := map[string]any{"content": "orig"}

	mustIndex(t, s, search.Document{ID: "m2", Index: "main", Content: "fresh words here", Metadata: meta})

	if meta["content"] != "orig" {
		t.Fatalf("caller metadata = %v, want orig preserved", meta)
	}

	res := mustSearch(t, s, "fresh", search.QueryOptions{Limit: 10})
	if len(res.Hits) != 1 || res.Hits[0].Metadata["content"] != "fresh words here" {
		t.Fatalf("Hits = %+v, want injected content to win", res.Hits)
	}
}

func TestUpdateOverwrite(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	mustIndex(t, s, search.Document{ID: "u1", Index: "main", Content: "alpha version"})
	mustIndex(t, s, search.Document{ID: "u1", Index: "main", Content: "beta version"})

	res := mustSearch(t, s, "beta", search.QueryOptions{Limit: 10})
	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "u1" {
		t.Fatalf("beta = %+v, want one hit u1", res)
	}

	res = mustSearch(t, s, "alpha", search.QueryOptions{Limit: 10})
	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("alpha = %+v, want empty after overwrite", res)
	}
}

func TestOperatorsNeutralized(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "p1", Index: "main", Content: "plain text here"})

	for _, q := range []string{"plain OR missing", "(unbalanced", "missing*", "title:plain", `"plain"`} {
		res, err := s.Search(ctx, q, search.QueryOptions{Limit: 10})
		if err != nil {
			t.Fatalf("Search(%q) err = %v, want no syntax error", q, err)
		}

		if q == `"plain"` {
			if res.Total != 1 {
				t.Fatalf("Search(%q) Total = %d, want 1", q, res.Total)
			}

			continue
		}

		if res.Total != 0 {
			t.Fatalf("Search(%q) Total = %d, want 0 (operators literal)", q, res.Total)
		}
	}

	res := mustSearch(t, s, "plain text", search.QueryOptions{Limit: 10})
	if res.Total != 1 {
		t.Fatalf("plain text Total = %d, want 1", res.Total)
	}

	// A trailing * stays a prefix operator inside FTS5 quotes: "plain*" is a
	// prefix phrase, not a literal. Quoting neutralizes operators that could
	// break syntax or widen logic (OR/AND/NOT, parens, colons), but it does
	// not suppress prefix expansion. Document the behavior here.
	res, err := s.Search(ctx, "plain*", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search(plain*) err = %v", err)
	}

	if res.Total != 1 {
		t.Fatalf("Search(plain*) Total = %d, want prefix match 1", res.Total)
	}
}

func TestFileBackedPersistence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "search.db")
	ctx := context.Background()

	s, err := New(search.Options{DSN: path})
	if err != nil {
		t.Fatal(err)
	}

	if indexErr := s.Index(ctx, search.Document{ID: "f1", Index: "main", Content: "persisted words"}); indexErr != nil {
		_ = s.Close()
		t.Fatal(indexErr)
	}

	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	s2, err := New(search.Options{DSN: path})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = s2.Close() }()

	res, err := s2.Search(ctx, "persisted", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "f1" {
		t.Fatalf("reopened = %+v, want one hit f1", res)
	}
}

func TestMemoryIsolation(t *testing.T) {
	t.Parallel()

	a := newMemoryStore(t)
	b := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, a, search.Document{ID: "only-a", Index: "main", Content: "solitary words"})

	res, err := b.Search(ctx, "solitary", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("second store = %+v, want isolated empty", res)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	if closeErr := s.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	ctx := context.Background()

	if indexErr := s.Index(ctx, search.Document{ID: "x", Index: "main", Content: "y"}); indexErr == nil {
		t.Fatal("expected error indexing closed store")
	}

	if deleteErr := s.Delete(ctx, "x"); deleteErr == nil {
		t.Fatal("expected error deleting from closed store")
	}

	res, err := s.Search(ctx, "y", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected error searching closed store")
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("closed search = %+v, want zero value", res)
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	const n = 20

	var wg sync.WaitGroup

	errs := make(chan error, 2*n)

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			id := fmt.Sprintf("c%d", i)
			if err := s.Index(ctx, search.Document{ID: id, Index: "main", Content: fmt.Sprintf("concurrent payload %d", i)}); err != nil {
				errs <- err
				return
			}

			if _, err := s.Search(ctx, "concurrent", search.QueryOptions{Limit: n}); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	res := mustSearch(t, s, "concurrent", search.QueryOptions{Limit: n})
	if res.Total != n {
		t.Fatalf("Total = %d, want %d", res.Total, n)
	}
}

func TestIndexUnserializableMetadata(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)

	err := s.Index(context.Background(), search.Document{
		ID:       "bad",
		Index:    "main",
		Content:  "content",
		Metadata: map[string]any{"f": func() {}},
	})
	if err == nil {
		t.Fatal("expected error for unserializable metadata")
	}

	if !strings.Contains(err.Error(), "sqlite: index") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: index")
	}
}

func TestIndexClosedDB(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	err = s.Index(context.Background(), search.Document{ID: "x", Index: "main", Content: "y"})
	if err == nil {
		t.Fatal("expected error indexing closed store")
	}

	if !strings.Contains(err.Error(), "sqlite: index") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: index")
	}
}

func TestIndexExecError(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER block_docs BEFORE INSERT ON search_documents BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}

	err := s.Index(ctx, search.Document{ID: "x", Index: "main", Content: "blocked words"})
	if err == nil {
		t.Fatal("expected error when docs insert fails")
	}

	if !strings.Contains(err.Error(), "sqlite: index") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: index")
	}
}

func TestIndexCancelledContext(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	err := s.Index(cancelCtx, search.Document{ID: "x", Index: "main", Content: "y"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDeleteExecError(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "doomed words"})

	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER block_del BEFORE DELETE ON search_documents BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}

	err := s.Delete(ctx, "d1")
	if err == nil {
		t.Fatal("expected error when delete fails")
	}

	if !strings.Contains(err.Error(), "sqlite: delete") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: delete")
	}
}

func TestDeleteCancelledContext(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "doomed words"})

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if err := s.Delete(cancelCtx, "d1"); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSearchClosedDB(t *testing.T) {
	t.Parallel()

	s, err := New(search.Options{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}

	if closeErr := s.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	res, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected error searching closed store")
	}

	if !strings.Contains(err.Error(), "sqlite: search") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search")
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("closed search = %+v, want zero value", res)
	}
}

func TestSearchCancelledContext(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "hello world"})

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := s.Search(cancelCtx, "hello", search.QueryOptions{Limit: 10}); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSearchCorruptMetadata(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_documents (id, idx, content, metadata) VALUES ('bad', 'main', 'broken meta words', X'7B6F6F7073')`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_fts(rowid, content, id, idx) SELECT rowid, content, id, idx FROM search_documents WHERE id = 'bad'`); err != nil {
		t.Fatal(err)
	}

	_, err := s.Search(ctx, "broken", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected error for corrupt metadata")
	}

	if !strings.Contains(err.Error(), "sqlite: search") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search")
	}
}

func TestSearchCountNoRows(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(query string) (driver.Rows, error) {
			if strings.Contains(query, "COUNT") {
				// Defensive branch: COUNT(*) always returns one row, so
				// empty rows can only come from a scripted driver.
				return &stubRows{cols: []string{"total"}}, nil
			}

			return hitRows(), nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	_, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected no-rows error for empty count")
	}

	if !strings.Contains(err.Error(), "sqlite: search: count: no rows") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search: count: no rows")
	}
}

func TestSearchHitsScanError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(query string) (driver.Rows, error) {
			if strings.Contains(query, "COUNT") {
				return &stubRows{cols: []string{"total"}, vals: [][]driver.Value{{int64(1)}}}, nil
			}

			// bool is not convertible into the float64 score destination,
			// so rows.Scan fails deterministically.
			return &stubRows{
				cols: []string{"id", "metadata", "score"},
				vals: [][]driver.Value{{"d1", []byte(`{"content":"x"}`), true}},
			}, nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	_, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected scan error for bool score")
	}

	if !strings.Contains(err.Error(), "sqlite: search: scan") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search: scan")
	}
}

func TestSearchNilMetadata(t *testing.T) {
	t.Parallel()

	s := newMemoryStore(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_documents (id, idx, content, metadata) VALUES ('nul', 'main', 'null meta words', NULL)`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.ExecContext(ctx, `INSERT INTO search_fts(rowid, content, id, idx) SELECT rowid, content, id, idx FROM search_documents WHERE id = 'nul'`); err != nil {
		t.Fatal(err)
	}

	res := mustSearch(t, s, "null", search.QueryOptions{Limit: 10})
	if len(res.Hits) != 1 || res.Hits[0].ID != "nul" {
		t.Fatalf("Hits = %+v, want one hit nul", res.Hits)
	}

	if res.Hits[0].Metadata != nil {
		t.Fatalf("Metadata = %v, want nil", res.Hits[0].Metadata)
	}
}

// errBoom is a sentinel for driver-failure injection tests.
var errBoom = errors.New("boom")

// stubConnector serves scripted driver behavior so commit, rows-affected,
// and row-iteration failure branches stay covered without a live failure.
type stubConnector struct {
	conn *stubConn
}

func (c stubConnector) Connect(context.Context) (driver.Conn, error) {
	return c.conn, nil
}

func (c stubConnector) Driver() driver.Driver { return stubDriver{} }

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, errBoom }

type stubConn struct {
	queryFn   func(query string) (driver.Rows, error)
	execFn    func(query string) (driver.Result, error)
	commitErr error
}

func (c *stubConn) Prepare(string) (driver.Stmt, error) { return nil, errBoom }
func (c *stubConn) Close() error                        { return nil }
func (c *stubConn) Begin() (driver.Tx, error)           { return stubTx{commitErr: c.commitErr}, nil }

func (c *stubConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	return c.execFn(query)
}

func (c *stubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return c.queryFn(query)
}

type stubTx struct {
	commitErr error
}

func (t stubTx) Commit() error   { return t.commitErr }
func (t stubTx) Rollback() error { return nil }

type stubResult struct {
	rows int64
	err  error
}

func (r stubResult) LastInsertId() (int64, error) { return 0, nil }
func (r stubResult) RowsAffected() (int64, error) { return r.rows, r.err }

type stubRows struct {
	cols    []string
	vals    [][]driver.Value
	pos     int
	nextErr error
}

func (r *stubRows) Columns() []string { return r.cols }
func (r *stubRows) Close() error      { return nil }

func (r *stubRows) Next(dest []driver.Value) error {
	if r.nextErr != nil {
		return r.nextErr
	}

	if r.pos >= len(r.vals) {
		return io.EOF
	}

	copy(dest, r.vals[r.pos])
	r.pos++

	return nil
}

func newStubStore(conn *stubConn) *Store {
	return &Store{db: sql.OpenDB(stubConnector{conn: conn})}
}

func hitRows() *stubRows {
	return &stubRows{
		cols: []string{"id", "metadata", "score"},
		vals: [][]driver.Value{{"d1", []byte(`{"content":"x"}`), float64(1.5)}},
	}
}

func TestIndexCommitError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		execFn:    func(string) (driver.Result, error) { return stubResult{rows: 1}, nil },
		queryFn:   func(string) (driver.Rows, error) { return nil, errBoom },
		commitErr: errBoom,
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	err := s.Index(context.Background(), search.Document{ID: "x", Index: "main", Content: "y"})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Index err = %v, want boom", err)
	}

	if !strings.Contains(err.Error(), "sqlite: index") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: index")
	}
}

func TestDeleteRowsAffectedError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		execFn:  func(string) (driver.Result, error) { return stubResult{err: errBoom}, nil },
		queryFn: func(string) (driver.Rows, error) { return nil, errBoom },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	err := s.Delete(context.Background(), "x")
	if !errors.Is(err, errBoom) {
		t.Fatalf("Delete err = %v, want boom", err)
	}
}

func TestDeleteCommitError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		execFn:    func(string) (driver.Result, error) { return stubResult{rows: 1}, nil },
		queryFn:   func(string) (driver.Rows, error) { return nil, errBoom },
		commitErr: errBoom,
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	err := s.Delete(context.Background(), "x")
	if !errors.Is(err, errBoom) {
		t.Fatalf("Delete err = %v, want boom", err)
	}
}

func TestSearchCountQueryError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(query string) (driver.Rows, error) {
			if strings.Contains(query, "COUNT") {
				return nil, errBoom
			}

			return hitRows(), nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	_, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Search err = %v, want boom", err)
	}

	if !strings.Contains(err.Error(), "sqlite: search") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search")
	}
}

func TestSearchCountScanError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(query string) (driver.Rows, error) {
			if strings.Contains(query, "COUNT") {
				return &stubRows{cols: []string{"total"}, vals: [][]driver.Value{{"junk"}}}, nil
			}

			return hitRows(), nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	_, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected scan error for junk count value")
	}

	if !strings.Contains(err.Error(), "sqlite: search") {
		t.Fatalf("err %q missing %q", err.Error(), "sqlite: search")
	}
}

func TestSearchCountRowsError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(query string) (driver.Rows, error) {
			if strings.Contains(query, "COUNT") {
				return &stubRows{cols: []string{"total"}, nextErr: errBoom}, nil
			}

			return hitRows(), nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	if _, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10}); !errors.Is(err, errBoom) {
		t.Fatalf("Search err = %v, want boom", err)
	}
}

func TestSearchHitsRowsError(t *testing.T) {
	t.Parallel()

	conn := &stubConn{
		queryFn: func(string) (driver.Rows, error) {
			return &stubRows{cols: []string{"id", "metadata", "score"}, nextErr: errBoom}, nil
		},
		execFn: func(string) (driver.Result, error) { return stubResult{}, nil },
	}
	s := newStubStore(conn)

	defer func() { _ = s.Close() }()

	if _, err := s.Search(context.Background(), "hello", search.QueryOptions{Limit: 10}); !errors.Is(err, errBoom) {
		t.Fatalf("Search err = %v, want boom", err)
	}
}

func TestBuildMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \t\n ", ""},
		{"single token", "hello", `"hello"`},
		{"implicit AND", "foo bar", `"foo" "bar"`},
		{"multi-space collapsed", "foo   bar", `"foo" "bar"`},
		{"quotes stripped", `say "hi"`, `"say" "hi"`},
		{"lone quote dropped", `"`, ``},
		{"quote-only token dropped", `a " b`, `"a" "b"`},
		{"dash kept literal", "foo-bar", `"foo-bar"`},
		{"special chars neutralized", `(a)*:b`, `"(a)*:b"`},
		{"keyword treated literal", "OR", `"OR"`},
		{"adjacent quote merged", `a"b`, `"ab"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := buildMatch(tc.query); got != tc.want {
				t.Errorf("buildMatch(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestValidateDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dsn     string
		wantErr bool
	}{
		{"", false},
		{":memory:", false},
		{"file.db", false},
		{"/tmp/zen.db", false},
		{"a\x00b", true},
		{"a\nb", true},
		{"a\rb", true},
		{"a;b", true},
		{"a;SELECT 1", true},
	}

	for _, tc := range cases {
		if err := validateDSN(tc.dsn); (err != nil) != tc.wantErr {
			t.Errorf("validateDSN(%q) err = %v, wantErr %v", tc.dsn, err, tc.wantErr)
		}
	}
}
