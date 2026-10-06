package db

import (
	"errors"
	"strings"
	"sync"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/orm/dialect"
)

func TestEdgeNewFromDB_nilDB(t *testing.T) {
	t.Parallel()

	_, err := NewFromDB(nil, Options{})
	if !errors.Is(err, ErrNilDB) {
		t.Fatalf("NewFromDB(nil) err = %v, want ErrNilDB", err)
	}
}

// TestEdgeDelete_success: deleting an existing document succeeds; a second
// delete reports NotFound.
func TestEdgeDelete_success(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "del", Index: "idx", Content: "ephemeral cedar"})

	if err := s.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete() err = %v, want nil", err)
	}

	if err := s.Delete(ctx, "del"); !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("Delete() second err = %v, want ErrNotFound", err)
	}
}

// TestEdgeSearch_filterNoMatch: an index filter matching nothing returns an
// empty result without error.
func TestEdgeSearch_filterNoMatch(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "f1", Index: "docs", Content: "needle in haystack"})

	res, err := s.Search(ctx, "needle", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "other"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("Search() = %+v, want empty", res)
	}
}

func TestEdgeNewFromDB_invalidOptions(t *testing.T) {
	t.Parallel()

	_, err := NewFromDB(nil, Options{Options: coredb.Options{MaxConns: -1}})
	if err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("NewFromDB(invalid) err = %v, want postgres-wrapped error", err)
	}
}

func TestEdgeNew_whitespaceDSN(t *testing.T) {
	t.Parallel()

	s, err := New(Options{Options: coredb.Options{DSN: "   "}})
	if err != nil {
		t.Fatalf("New(whitespace DSN) err = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	if d.conn.Dialect() != "sqlite" {
		t.Fatalf("Dialect() = %q, want sqlite", d.conn.Dialect())
	}
}

func TestEdgeIndexBatch_empty(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	if err := s.IndexBatch(t.Context(), nil); err != nil {
		t.Fatalf("IndexBatch(nil) err = %v, want nil", err)
	}

	if err := s.IndexBatch(t.Context(), []search.Document{}); err != nil {
		t.Fatalf("IndexBatch(empty) err = %v, want nil", err)
	}
}

func TestEdgeIndexBatch_invalidDocAtIndex(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

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

	s := mustOpenMemory(t)

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

func TestEdgeSearch_negativeLimit(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "n1", Index: "neg", Content: "negative limit probe"})

	res, err := s.Search(ctx, "negative", search.QueryOptions{Limit: -1, Filters: map[string]string{"index": "neg"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 {
		t.Fatalf("Search() = %+v, want one hit", res)
	}
}

func TestEdgeSearch_negativeOffset(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "o1", Index: "off", Content: "negative offset probe"})

	res, err := s.Search(ctx, "negative", search.QueryOptions{Limit: 10, Offset: -5, Filters: map[string]string{"index": "off"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 {
		t.Fatalf("Search() = %+v, want one hit", res)
	}
}

func TestEdgeSearch_nilFilters(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "f1", Index: "a", Content: "nil filter probe"})
	mustIndex(t, s, search.Document{ID: "f2", Index: "b", Content: "nil filter probe"})

	res, err := s.Search(ctx, "nil", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 2 {
		t.Fatalf("Search().Total = %d, want 2 (spans all indexes)", res.Total)
	}
}

func TestEdgeSearch_emptyIndexFilter(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "e1", Index: "x", Content: "edgeemptyfilterprobe"})
	mustIndex(t, s, search.Document{ID: "e2", Index: "y", Content: "edgeemptyfilterprobe"})

	res, err := s.Search(ctx, "edgeemptyfilterprobe", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": ""}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 2 {
		t.Fatalf("Search().Total = %d, want 2 (spans all indexes)", res.Total)
	}
}

func TestEdgeSearch_emptyQuery(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "q1", Index: "eq", Content: "empty query probe"})

	res, err := s.Search(ctx, "", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "eq"}})
	if err != nil {
		t.Fatalf("Search() err = %v", err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("Search(\"\") = %+v, want empty", res)
	}
}

func TestEdgeCoerceMeta_unsupported(t *testing.T) {
	t.Parallel()

	_, err := coerceMeta(42)
	if !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("coerceMeta(42) err = %v, want ErrInvalidMetadata", err)
	}
}

func TestEdgeDecodeHitMeta_malformed(t *testing.T) {
	t.Parallel()

	_, err := decodeHitMeta("not-json")
	if err == nil {
		t.Fatal("decodeHitMeta(\"not-json\") err = nil, want error")
	}
}

func TestEdgeBuildMatch_specialChars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"parens", "a(b)c", `"a(b)c"`},
		{"colon", "field:value", `"field:value"`},
		{"asterisk", "foo*", `"foo*"`},
		{"mixed", `a"b"c`, `"abc"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildMatch(tt.query); got != tt.want {
				t.Fatalf("buildMatch(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestEdgeBuildSearchQueries(t *testing.T) {
	t.Parallel()

	hitsSQL, countSQL, hitsArgs, countArgs := buildSearchQueries("q", map[string]string{"index": "docs"}, 10, 5)

	if !strings.Contains(hitsSQL, "LIMIT $3") || !strings.Contains(hitsSQL, "OFFSET $4") {
		t.Fatalf("hitsSQL = %q, want LIMIT $3 OFFSET $4", hitsSQL)
	}

	if !strings.Contains(countSQL, "COUNT(*)") {
		t.Fatalf("countSQL = %q, want COUNT(*)", countSQL)
	}

	if len(hitsArgs) != 4 {
		t.Fatalf("hitsArgs = %v, want 4 args", hitsArgs)
	}

	if len(countArgs) != 2 {
		t.Fatalf("countArgs = %v, want 2 args", countArgs)
	}
}

func TestEdgeDriver_concurrent(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

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

			if err := d.Index(ctx, doc); err != nil {
				t.Errorf("Index() err = %v", err)

				return
			}

			if _, err := d.Search(ctx, "concurrent", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "conc"}}); err != nil {
				t.Errorf("Search() err = %v", err)

				return
			}

			if err := d.Delete(ctx, doc.ID); err != nil {
				t.Errorf("Delete() err = %v", err)
			}
		}(i)
	}

	wg.Wait()
}

func TestEdgeClose_borrowedConn(t *testing.T) {
	t.Parallel()

	inner, err := New(Options{})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	t.Cleanup(func() { _ = inner.Close() })

	d, ok := inner.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", inner)
	}

	borrowed, err := NewFromDB(d.conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB() err = %v", err)
	}

	if err := borrowed.Close(); err != nil {
		t.Fatalf("Close() err = %v", err)
	}

	if err := d.conn.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() after borrowed Close() err = %v, want usable conn", err)
	}
}

func TestEdgeNewFromDB_unsupportedDialect(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	stub := &stubDB{DB: conn, dialect: "mysql"}

	if _, err := NewFromDB(stub, Options{}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("NewFromDB(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}
}
