package postgres

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// stubDB is a coredb.DB double with a scripted dialect.
type stubDB struct {
	coredb.DB
	dialect string
}

func (s *stubDB) Dialect() string { return s.dialect }

func mustOpenMemory(t *testing.T) search.Search {
	t.Helper()

	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func mustIndex(t *testing.T, s search.Search, doc search.Document) {
	t.Helper()

	if err := s.Index(t.Context(), doc); err != nil {
		t.Fatalf("Index(%q) error = %v", doc.ID, err)
	}
}

func TestOptions_Validate(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	err := Options{Options: coredb.Options{MaxConns: -1}}.Validate()
	if err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("Validate() error = %v, want postgres-wrapped error", err)
	}
}

func TestIsPostgresDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dsn  string
		want bool
	}{
		{"", false},
		{":memory:", false},
		{"file.db", false},
		{"/tmp/x.db", false},
		{"postgres://localhost:5432/zever", true},
		{"postgresql://localhost:5432/zever", true},
		{"  POSTGRES://h/db  ", true},
		{"mysql://h/db", false},
		{"://bad", false},
	}

	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			t.Parallel()

			if got := dbconn.IsPostgresDSN(tt.dsn); got != tt.want {
				t.Fatalf("IsPostgresDSN(%q) = %v, want %v", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestDbOptions(t *testing.T) {
	t.Parallel()

	if got := dbconn.SplitDSN(""); got.Path != ":memory:" || got.DSN != "" {
		t.Fatalf("SplitDSN(\"\") = %+v, want sqlite :memory:", got)
	}

	if got := dbconn.SplitDSN("file.db"); got.Path != "file.db" || got.DSN != "" {
		t.Fatalf("SplitDSN(file.db) = %+v, want Path", got)
	}

	if got := dbconn.SplitDSN("postgres://h/db"); got.DSN != "postgres://h/db" || got.Path != "" {
		t.Fatalf("SplitDSN(postgres) = %+v, want DSN", got)
	}
}

func TestNew_emptySelectsSQLite(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	if d.conn.Dialect() != "sqlite" {
		t.Fatalf("Dialect() = %q, want sqlite", d.conn.Dialect())
	}

	if !d.owns {
		t.Fatal("owns = false, want true for New-built driver")
	}

	ctx := t.Context()
	mustIndex(t, s, search.Document{ID: "d1", Index: "main", Content: "hello world"})

	res, err := s.Search(ctx, "hello", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "main"}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "d1" {
		t.Fatalf("Search() = %+v, want one hit d1", res)
	}
}

func TestNew_sqliteFilePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "search.db")

	s, err := New(Options{Options: coredb.Options{Path: path}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
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

func TestNew_nonPostgresDSNIsSQLitePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dsn.db")

	s, err := New(Options{Options: coredb.Options{DSN: path}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	if d, ok := s.(*driver); !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	} else if d.conn.Dialect() != "sqlite" {
		t.Fatalf("Dialect() = %q, want sqlite", d.conn.Dialect())
	}
}

// TestNew_postgresRefused proves DSN routing: a postgres URL reaches
// dbpostgres.New, whose TLS-1.2-flooring constructor then fails dialing the
// closed port. The error (not the sqlite path) proves the pgx config
// branch ran; see adapters/db/postgres/postgres.go:342-351.
func TestNew_postgresRefused(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: coredb.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=disable"}})
	if err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("New() error = %v, want postgres-wrapped dial error", err)
	}
}

func TestNew_postgresInvalidDSN(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: coredb.Options{DSN: "postgres://[::1"}})
	if err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("New() error = %v, want postgres-wrapped config error", err)
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: coredb.Options{MaxConns: -1}})
	if err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("New() error = %v, want postgres-wrapped error", err)
	}
}

func TestNewFromDB_nil(t *testing.T) {
	t.Parallel()

	if _, err := NewFromDB(nil, Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

func TestNewFromDB_borrowsConn(t *testing.T) {
	t.Parallel()

	inner, err := New(Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = inner.Close() })

	d, ok := inner.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", inner)
	}

	borrowed, err := NewFromDB(d.conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB() error = %v", err)
	}

	bd, ok := borrowed.(*driver)
	if !ok {
		t.Fatalf("NewFromDB() returned %T, want *driver", borrowed)
	}

	if bd.owns {
		t.Fatal("owns = true, want false for borrowed connection")
	}

	mustIndex(t, borrowed, search.Document{ID: "b1", Index: "idx", Content: "borrowed probe"})

	// Borrowed connection: driver Close must not close the injected DB.
	if err := borrowed.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := d.conn.Ping(t.Context()); err != nil {
		t.Fatalf("injected DB closed by driver Close: %v", err)
	}
}

func TestCheckDialect_failsClosed(t *testing.T) {
	t.Parallel()

	d := &driver{conn: &stubDB{dialect: "mysql"}}

	if err := d.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect() error = %v, want unsupported-dialect error", err)
	}
}

func TestIndex_invalidMetadata(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	bad := search.Document{
		ID:       "bad",
		Index:    "idx",
		Content:  "probe",
		Metadata: map[string]any{"ch": make(chan int)},
	}

	if err := s.Index(t.Context(), bad); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("Index() error = %v, want ErrInvalidMetadata", err)
	}

	if err := s.IndexBatch(t.Context(), []search.Document{bad}); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("IndexBatch() error = %v, want ErrInvalidMetadata", err)
	}
}

func TestDelete_missing(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	err := s.Delete(t.Context(), "ghost")
	if err == nil {
		t.Fatal("Delete() error = nil, want NotFoundError")
	}

	var nf *search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Delete() error = %v, want *NotFoundError", err)
	}

	if nf.ID != "ghost" {
		t.Fatalf("NotFoundError.ID = %q, want ghost", nf.ID)
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSearchRoundTrip_sqlite(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	const idx = "rt"

	mustIndex(t, s, search.Document{ID: "a", Index: idx, Content: "conformancealpha bright meadow"})
	mustIndex(t, s, search.Document{ID: "b", Index: idx, Content: "other silent harbor"})

	res, err := s.Search(ctx, "conformancealpha", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "a" {
		t.Fatalf("Search() = %+v, want hit a", res)
	}

	if res.Hits[0].Metadata["content"] != "conformancealpha bright meadow" {
		t.Fatalf("hit metadata = %v", res.Hits[0].Metadata)
	}

	// Overwrite replaces: the old term must vanish, total stays 1.
	mustIndex(t, s, search.Document{ID: "a", Index: idx, Content: "replaced sequoia"})

	res, err = s.Search(ctx, "conformancealpha", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("Search() = %+v, want empty after overwrite", res)
	}

	// Delete round trip.
	if derr := s.Delete(ctx, "a"); derr != nil {
		t.Fatalf("Delete() error = %v", derr)
	}

	res, err = s.Search(ctx, "replaced", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 0 {
		t.Fatalf("Search().Total = %d, want 0 after delete", res.Total)
	}

	if derr := s.Delete(ctx, "a"); !errors.Is(derr, search.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", derr)
	}
}

func TestSearch_defaultsAndPaging(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustIndex(t, s, search.Document{ID: "l1", Index: "lim", Content: "paging quiet orchard"})

	res, err := s.Search(ctx, "paging", search.QueryOptions{Filters: map[string]string{"index": "lim"}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 {
		t.Fatalf("Search() = %+v, want default-limit hit", res)
	}

	res, err = s.Search(ctx, "paging", search.QueryOptions{Limit: 10, Offset: 100, Filters: map[string]string{"index": "lim"}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 {
		t.Fatalf("Search().Total = %d, want 1", res.Total)
	}

	if len(res.Hits) != 0 {
		t.Fatalf("Search().Hits = %v, want none beyond total", res.Hits)
	}

	res, err = s.Search(ctx, "   ", search.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("Search(blank) = %+v, want empty", res)
	}
}

func TestIndexBatch_replacesEachRow(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	docs := []search.Document{
		{ID: "m1", Index: "mb", Content: "first pine forest"},
		{ID: "m2", Index: "mb", Content: "second pine forest"},
	}
	if err := s.IndexBatch(ctx, docs); err != nil {
		t.Fatalf("IndexBatch() error = %v", err)
	}

	// Re-index m1 with new content: conflict replacement must carry m1's
	// own new content, not a sibling row's.
	docs[0].Content = "first oak desert"

	if err := s.IndexBatch(ctx, docs); err != nil {
		t.Fatalf("IndexBatch() error = %v", err)
	}

	res, err := s.Search(ctx, "pine", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "mb"}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "m2" {
		t.Fatalf("Search(pine) = %+v, want only m2", res)
	}

	res, err = s.Search(ctx, "oak", search.QueryOptions{Limit: 10, Filters: map[string]string{"index": "mb"}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "m1" {
		t.Fatalf("Search(oak) = %+v, want only m1", res)
	}
}

func TestBuildMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"single", "hello", `"hello"`},
		{"multi", "hello world", `"hello" "world"`},
		{"quotes stripped", `a"b`, `"ab"`},
		{"keywords quoted", "AND OR", `"AND" "OR"`},
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

func TestMatchWhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		filters map[string]string
		where   string
		args    []any
	}{
		{"nil", nil, " WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)", []any{"q"}},
		{"index", map[string]string{"index": "docs"}, " WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1) AND idx = $2", []any{"q", "docs"}},
		{"empty index", map[string]string{"index": ""}, " WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)", []any{"q"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			where, args := matchWhere("q", tt.filters)
			if where != tt.where {
				t.Fatalf("where = %q, want %q", where, tt.where)
			}

			if len(args) != len(tt.args) {
				t.Fatalf("args = %v, want %v", args, tt.args)
			}

			for i := range tt.args {
				if args[i] != tt.args[i] {
					t.Fatalf("args = %v, want %v", args, tt.args)
				}
			}
		})
	}
}

func TestClose_idempotent(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() second error = %v, want nil", err)
	}
}

func TestOpenRegister_bothNames(t *testing.T) {
	Register()

	for _, adapter := range []search.Adapter{search.Postgres, search.SQLite} {
		s, err := search.Open(adapter, search.Options{})
		if err != nil {
			t.Fatalf("Open(%s) error = %v", adapter, err)
		}

		if err := s.Index(t.Context(), search.Document{ID: "w", Index: "kit", Content: "wiring probe"}); err != nil {
			t.Errorf("Open(%s) Index() error = %v", adapter, err)
		}

		_ = s.Close()
	}
}

func TestLive_postgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("SEARCH_PG_DSN")
	}

	if dsn == "" {
		t.Skip("POSTGRES_DSN/SEARCH_PG_DSN not set")
	}

	ctx := t.Context()

	s, err := New(Options{Options: coredb.Options{DSN: dsn}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	const idx = "e2e-search-db"

	docs := []search.Document{
		{ID: "e2e-run-once", Index: idx, Content: "going for a run in the park", Metadata: map[string]any{"n": 1}},
		{ID: "e2e-run-many", Index: idx, Content: "run run run running fast every morning", Metadata: map[string]any{"n": 2}},
	}

	if berr := s.IndexBatch(ctx, docs); berr != nil {
		t.Fatalf("IndexBatch() error = %v", berr)
	}

	t.Cleanup(func() {
		for _, d := range docs {
			_ = s.Delete(ctx, d.ID)
		}
	})

	// Stemming: "running" matches the document containing only "run".
	res, err := s.Search(ctx, "running", search.QueryOptions{Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 2 {
		t.Fatalf("Total = %d, want 2", res.Total)
	}

	// Rank ordering: the document repeating the term ranks first.
	if len(res.Hits) != 2 || res.Hits[0].ID != "e2e-run-many" {
		t.Fatalf("Hits = %+v, want e2e-run-many first", res.Hits)
	}

	if derr := s.Delete(ctx, "e2e-run-once"); derr != nil {
		t.Fatalf("Delete() error = %v", derr)
	}

	res, err = s.Search(ctx, "running", search.QueryOptions{Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "e2e-run-many" {
		t.Fatalf("after delete = %+v", res)
	}
}
