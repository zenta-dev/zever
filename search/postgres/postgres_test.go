package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zenta-dev/zever/search"
)

// fakeRows is a scripted pgx.Rows implementation for unit tests.
type fakeRows struct {
	rows    [][]any
	cur     int
	scanErr error
	rowErr  error
	closed  bool
}

func (r *fakeRows) Close() { r.closed = true }

func (r *fakeRows) Err() error { return r.rowErr }

func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.NewCommandTag("SELECT 0") }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	if r.cur < len(r.rows) {
		r.cur++

		return true
	}

	return false
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	vals := r.rows[r.cur-1]
	if len(dest) != len(vals) {
		return fmt.Errorf("fakeRows: dest %d != vals %d", len(dest), len(vals))
	}

	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			v, ok := vals[i].(string)
			if !ok {
				return fmt.Errorf("fakeRows: col %d not a string", i)
			}

			*p = v
		case *[]byte:
			if vals[i] == nil {
				*p = nil
			} else {
				v, ok := vals[i].([]byte)
				if !ok {
					return fmt.Errorf("fakeRows: col %d not bytes", i)
				}

				*p = v
			}
		case *int64:
			v, ok := vals[i].(int64)
			if !ok {
				return fmt.Errorf("fakeRows: col %d not an int64", i)
			}

			*p = v
		case *float64:
			v, ok := vals[i].(float64)
			if !ok {
				return fmt.Errorf("fakeRows: col %d not a float64", i)
			}

			*p = v
		default:
			return fmt.Errorf("fakeRows: unsupported dest %T", d)
		}
	}

	return nil
}

func (r *fakeRows) Values() ([]any, error) {
	if r.cur < 1 || r.cur > len(r.rows) {
		return nil, r.rowErr
	}

	return r.rows[r.cur-1], nil
}

func (r *fakeRows) RawValues() [][]byte { return nil }

func (r *fakeRows) Conn() *pgx.Conn { return nil }

// fakePool is a scripted dbpool implementation for unit tests.
type fakePool struct {
	execSQLs  []string
	execArgs  [][]any
	execTag   pgconn.CommandTag
	execErr   error
	execFail  string // fail Exec when SQL contains this substring
	queries   []string
	queryArgs [][]any
	hitsRows  pgx.Rows
	countRows pgx.Rows
	queryErr  error
	countErr  error
	closed    bool
}

func (f *fakePool) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execSQLs = append(f.execSQLs, sql)
	f.execArgs = append(f.execArgs, args)

	if f.execErr != nil && (f.execFail == "" || strings.Contains(sql, f.execFail)) {
		return pgconn.CommandTag{}, f.execErr
	}

	return f.execTag, nil
}

func (f *fakePool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queries = append(f.queries, sql)
	f.queryArgs = append(f.queryArgs, args)

	if strings.Contains(sql, "COUNT(*)") {
		if f.countErr != nil {
			return nil, f.countErr
		}

		return f.countRows, nil
	}

	if f.queryErr != nil {
		return nil, f.queryErr
	}

	return f.hitsRows, nil
}

func (f *fakePool) Close() { f.closed = true }

func stubPool(t *testing.T, pool dbpool) {
	t.Helper()

	orig := newPool
	newPool = func(context.Context, string) (dbpool, error) { return pool, nil }
	t.Cleanup(func() { newPool = orig })
}

func TestErrors_messages(t *testing.T) {
	t.Parallel()

	if got := ErrMissingDSN.Error(); got != "postgres: dsn is required" {
		t.Fatalf("ErrMissingDSN = %q", got)
	}

	if got := ErrNotConfigured.Error(); got != "postgres: not configured (missing DSN)" {
		t.Fatalf("ErrNotConfigured = %q", got)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(search.Options{Host: "http://"})
	if !errors.Is(err, search.ErrInvalidOptions) {
		t.Fatalf("Open invalid opts err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpen_devNoop(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	s, err := New(search.Options{})
	if err != nil {
		t.Fatalf("Open empty DSN err = %v", err)
	}

	if err := s.Index(ctx, search.Document{ID: "d", Index: "i", Content: "c"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Index err = %v, want ErrNotConfigured", err)
	}

	if err := s.Delete(ctx, "d"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Delete err = %v, want ErrNotConfigured", err)
	}

	if _, err := s.Search(ctx, "q", search.QueryOptions{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Search err = %v, want ErrNotConfigured", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

// The Open tests below stub the pool-constructor seam, so they run sequentially.

func TestOpen_poolError(t *testing.T) {
	orig := newPool
	newPool = func(context.Context, string) (dbpool, error) { return nil, errors.New("dial boom") }
	t.Cleanup(func() { newPool = orig })

	if _, err := New(search.Options{DSN: "postgres://db"}); err == nil || !strings.Contains(err.Error(), "postgres:") {
		t.Fatalf("Open pool err = %v, want postgres-wrapped error", err)
	}
}

func TestOpen_createTableError(t *testing.T) {
	fp := &fakePool{execErr: errors.New("table boom"), execFail: "CREATE TABLE"}
	stubPool(t, fp)

	if _, err := New(search.Options{DSN: "postgres://db"}); err == nil || !strings.Contains(err.Error(), "create table") {
		t.Fatalf("Open table err = %v, want create-table error", err)
	}

	if !fp.closed {
		t.Fatal("pool not closed on create-table error")
	}
}

func TestOpen_createIndexError(t *testing.T) {
	fp := &fakePool{execErr: errors.New("index boom"), execFail: "CREATE INDEX"}
	stubPool(t, fp)

	if _, err := New(search.Options{DSN: "postgres://db"}); err == nil || !strings.Contains(err.Error(), "create index") {
		t.Fatalf("Open index err = %v, want create-index error", err)
	}

	if !fp.closed {
		t.Fatal("pool not closed on create-index error")
	}
}

func TestOpen_successRunsDDL(t *testing.T) {
	fp := &fakePool{execTag: pgconn.NewCommandTag("CREATE TABLE")}
	stubPool(t, fp)

	s, err := New(search.Options{DSN: "postgres://db"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if len(fp.execSQLs) != 2 {
		t.Fatalf("DDL execs = %d, want 2", len(fp.execSQLs))
	}

	if !strings.Contains(fp.execSQLs[0], "CREATE TABLE") || !strings.Contains(fp.execSQLs[0], "search_documents") {
		t.Fatalf("table DDL = %q", fp.execSQLs[0])
	}

	if !strings.Contains(fp.execSQLs[1], "CREATE INDEX") || !strings.Contains(fp.execSQLs[1], "to_tsvector") {
		t.Fatalf("index DDL = %q", fp.execSQLs[1])
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if !fp.closed {
		t.Fatal("pool not closed")
	}
}

func TestIndex_ok(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fp := &fakePool{execTag: pgconn.NewCommandTag("INSERT 0 1")}
	p := &postgres{db: fp}

	meta := map[string]any{"k": "v"}
	doc := search.Document{ID: "d1", Index: "docs", Content: "hello world", Metadata: meta}

	if err := p.Index(ctx, doc); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	if len(fp.execSQLs) != 1 {
		t.Fatalf("execs = %d, want 1", len(fp.execSQLs))
	}

	sql := fp.execSQLs[0]
	for _, sub := range []string{"INSERT INTO search_documents", "$1", "$6", "ON CONFLICT (id, idx)"} {
		if !strings.Contains(sql, sub) {
			t.Fatalf("index SQL = %q, want substring %q", sql, sub)
		}
	}

	args := fp.execArgs[0]
	if len(args) != 6 {
		t.Fatalf("args = %d, want 6", len(args))
	}

	if args[0] != "d1" || args[1] != "docs" || args[2] != "hello world" || args[4] != "hello world" {
		t.Fatalf("args = %v", args)
	}

	rawMeta, ok := args[3].([]byte)
	if !ok {
		t.Fatalf("metadata arg = %T, want []byte", args[3])
	}

	var stored map[string]any
	if err := json.Unmarshal(rawMeta, &stored); err != nil {
		t.Fatalf("metadata JSON err = %v", err)
	}

	if stored["content"] != "hello world" || stored["k"] != "v" {
		t.Fatalf("stored metadata = %v", stored)
	}

	if _, ok := meta["content"]; ok {
		t.Fatal("caller metadata mutated")
	}
}

func TestIndex_execError(t *testing.T) {
	t.Parallel()

	fp := &fakePool{execErr: errors.New("exec boom")}
	p := &postgres{db: fp}

	err := p.Index(t.Context(), search.Document{ID: "d", Index: "i", Content: "c"})
	if err == nil || !strings.Contains(err.Error(), "postgres: index") {
		t.Fatalf("Index err = %v, want postgres index error", err)
	}
}

func TestIndex_metadataMarshalError(t *testing.T) {
	t.Parallel()

	p := &postgres{db: &fakePool{}}

	err := p.Index(t.Context(), search.Document{ID: "d", Metadata: map[string]any{"fn": func() {}}})
	if err == nil || !strings.Contains(err.Error(), "postgres: index") {
		t.Fatalf("Index err = %v, want postgres index error", err)
	}
}

func TestDelete_found(t *testing.T) {
	t.Parallel()

	fp := &fakePool{execTag: pgconn.NewCommandTag("DELETE 1")}
	p := &postgres{db: fp}

	if err := p.Delete(t.Context(), "d1"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	if !strings.Contains(fp.execSQLs[0], "DELETE FROM search_documents") || !strings.Contains(fp.execSQLs[0], "$1") {
		t.Fatalf("delete SQL = %q", fp.execSQLs[0])
	}

	if len(fp.execArgs[0]) != 1 || fp.execArgs[0][0] != "d1" {
		t.Fatalf("delete args = %v", fp.execArgs[0])
	}
}

func TestDelete_missing(t *testing.T) {
	t.Parallel()

	fp := &fakePool{execTag: pgconn.NewCommandTag("DELETE 0")}
	p := &postgres{db: fp}

	err := p.Delete(t.Context(), "ghost")
	if err == nil {
		t.Fatal("Delete err = nil, want NotFoundError")
	}

	var nf *search.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Delete err = %v, want NotFoundError", err)
	}

	if nf.ID != "ghost" {
		t.Fatalf("NotFoundError ID = %q", nf.ID)
	}

	if !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("Delete err = %v, want ErrNotFound", err)
	}
}

func TestDelete_execError(t *testing.T) {
	t.Parallel()

	fp := &fakePool{execErr: errors.New("exec boom")}
	p := &postgres{db: fp}

	err := p.Delete(t.Context(), "d")
	if err == nil || !strings.Contains(err.Error(), "postgres: delete") {
		t.Fatalf("Delete err = %v, want postgres delete error", err)
	}
}

func hitsPool(hits [][]any, total int64) *fakePool {
	return &fakePool{
		hitsRows:  &fakeRows{rows: hits},
		countRows: &fakeRows{rows: [][]any{{total}}},
	}
}

func TestSearch_defaults(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	p := &postgres{db: fp}

	res, err := p.Search(t.Context(), "hello", search.QueryOptions{})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if res.Total != 0 || len(res.Hits) != 0 {
		t.Fatalf("res = %+v", res)
	}

	if len(fp.queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(fp.queries))
	}

	hitsArgs := fp.queryArgs[0]
	if hitsArgs[len(hitsArgs)-2] != search.DefaultLimit || hitsArgs[len(hitsArgs)-1] != 0 {
		t.Fatalf("default limit/offset args = %v", hitsArgs)
	}
}

func TestSearch_limitOffset(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{Limit: 5, Offset: 7, Filters: map[string]string{"index": "docs"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	hitsArgs := fp.queryArgs[0]
	want := []any{"q", "docs", 5, 7}
	if len(hitsArgs) != len(want) {
		t.Fatalf("hits args = %v, want %v", hitsArgs, want)
	}

	for i := range want {
		if hitsArgs[i] != want[i] {
			t.Fatalf("hits args = %v, want %v", hitsArgs, want)
		}
	}

	if got := fp.queryArgs[1]; len(got) != 2 || got[0] != "q" || got[1] != "docs" {
		t.Fatalf("count args = %v", got)
	}
}

func TestSearch_negativeOffsetClamped(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{Limit: -3, Offset: -2})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	args := fp.queryArgs[0]
	if args[len(args)-2] != search.DefaultLimit || args[len(args)-1] != 0 {
		t.Fatalf("clamped args = %v", args)
	}
}

func TestSearch_hitsMapping(t *testing.T) {
	t.Parallel()

	fp := hitsPool([][]any{
		{"doc-1", []byte(`{"content":"hello","k":"v"}`), 0.75},
		{"doc-2", nil, 0.25},
	}, 2)
	p := &postgres{db: fp}

	res, err := p.Search(t.Context(), "hello", search.QueryOptions{Filters: map[string]string{"index": "docs"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}

	if len(res.Hits) != 2 {
		t.Fatalf("hits = %+v", res.Hits)
	}

	if res.Hits[0].ID != "doc-1" || res.Hits[0].Score != 0.75 {
		t.Fatalf("hit0 = %+v", res.Hits[0])
	}

	if res.Hits[0].Metadata["k"] != "v" {
		t.Fatalf("hit0 metadata = %v", res.Hits[0].Metadata)
	}

	if res.Hits[1].ID != "doc-2" || res.Hits[1].Metadata != nil {
		t.Fatalf("hit1 = %+v", res.Hits[1])
	}

	for _, sub := range []string{"ts_rank", "ORDER BY score DESC", "LIMIT $3 OFFSET $4"} {
		if !strings.Contains(fp.queries[0], sub) {
			t.Fatalf("hits SQL = %q, want substring %q", fp.queries[0], sub)
		}
	}

	if !strings.Contains(fp.queries[1], "SELECT COUNT(*)") {
		t.Fatalf("count SQL = %q", fp.queries[1])
	}
}

func TestSearch_hitsQueryError(t *testing.T) {
	t.Parallel()

	fp := &fakePool{queryErr: errors.New("query boom")}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search:") {
		t.Fatalf("Search err = %v, want postgres search error", err)
	}
}

func TestSearch_scanError(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.hitsRows = &fakeRows{rows: [][]any{{"d", []byte(`{}`), 0.1}}, scanErr: errors.New("scan boom")}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search scan") {
		t.Fatalf("Search err = %v, want scan error", err)
	}
}

func TestSearch_badMetadataJSON(t *testing.T) {
	t.Parallel()

	fp := hitsPool([][]any{{"d", []byte(`{bad`), 0.1}}, 1)
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search scan") {
		t.Fatalf("Search err = %v, want scan error", err)
	}
}

func TestSearch_hitsRowsError(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.hitsRows = &fakeRows{rowErr: errors.New("rows boom")}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search:") {
		t.Fatalf("Search err = %v, want postgres search error", err)
	}
}

func TestSearch_countError(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.countErr = errors.New("count boom")
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search count") {
		t.Fatalf("Search err = %v, want count error", err)
	}
}

func TestSearch_countRowsError(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.countRows = &fakeRows{rowErr: errors.New("count rows boom")}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search count") {
		t.Fatalf("Search err = %v, want count error", err)
	}
}

func TestSearch_countNoRows(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.countRows = &fakeRows{}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "no rows") {
		t.Fatalf("Search err = %v, want no-rows error", err)
	}
}

func TestSearch_countScanError(t *testing.T) {
	t.Parallel()

	fp := hitsPool(nil, 0)
	fp.countRows = &fakeRows{rows: [][]any{{"bad"}}}
	p := &postgres{db: fp}

	_, err := p.Search(t.Context(), "q", search.QueryOptions{})
	if err == nil || !strings.Contains(err.Error(), "postgres: search count") {
		t.Fatalf("Search err = %v, want count error", err)
	}
}

func TestMatchWhere(t *testing.T) {
	t.Parallel()

	whereNoFilter := " WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)"
	whereIndex := whereNoFilter + " AND idx = $2"

	tests := []struct {
		name    string
		query   string
		filters map[string]string
		where   string
		args    []any
	}{
		{"no filter", "hello", nil, whereNoFilter, []any{"hello"}},
		{"empty filters", "hello", map[string]string{}, whereNoFilter, []any{"hello"}},
		{"index filter", "hello", map[string]string{"index": "docs"}, whereIndex, []any{"hello", "docs"}},
		{"empty index ignored", "hello", map[string]string{"index": ""}, whereNoFilter, []any{"hello"}},
		{"other key ignored", "hello", map[string]string{"foo": "bar"}, whereNoFilter, []any{"hello"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			where, args := matchWhere(tt.query, tt.filters)
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

func TestBuildSearchQueries(t *testing.T) {
	t.Parallel()

	t.Run("no filter", func(t *testing.T) {
		t.Parallel()

		hitsSQL, countSQL, hitsArgs, countArgs := buildSearchQueries("hello", nil, 10, 0)

		wantCount := "SELECT COUNT(*) FROM search_documents" +
			" WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)"
		if countSQL != wantCount {
			t.Fatalf("countSQL = %q, want %q", countSQL, wantCount)
		}

		wantHits := "SELECT id, metadata, " +
			"ts_rank(to_tsvector('english', content), plainto_tsquery('english', $1)) AS score" +
			" FROM search_documents" +
			" WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)" +
			" ORDER BY score DESC LIMIT $2 OFFSET $3"
		if hitsSQL != wantHits {
			t.Fatalf("hitsSQL = %q, want %q", hitsSQL, wantHits)
		}

		assertArgs(t, hitsArgs, []any{"hello", 10, 0})
		assertArgs(t, countArgs, []any{"hello"})
	})

	t.Run("index filter", func(t *testing.T) {
		t.Parallel()

		hitsSQL, countSQL, hitsArgs, countArgs := buildSearchQueries("hello", map[string]string{"index": "docs"}, 5, 7)

		wantCount := "SELECT COUNT(*) FROM search_documents" +
			" WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1) AND idx = $2"
		if countSQL != wantCount {
			t.Fatalf("countSQL = %q, want %q", countSQL, wantCount)
		}

		wantHits := "SELECT id, metadata, " +
			"ts_rank(to_tsvector('english', content), plainto_tsquery('english', $1)) AS score" +
			" FROM search_documents" +
			" WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1) AND idx = $2" +
			" ORDER BY score DESC LIMIT $3 OFFSET $4"
		if hitsSQL != wantHits {
			t.Fatalf("hitsSQL = %q, want %q", hitsSQL, wantHits)
		}

		assertArgs(t, hitsArgs, []any{"hello", "docs", 5, 7})
		assertArgs(t, countArgs, []any{"hello", "docs"})
	})
}

func assertArgs(t *testing.T, got, want []any) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	if err := (&postgres{db: nil}).Close(); err != nil {
		t.Fatalf("nil Close err = %v", err)
	}

	fp := &fakePool{}
	if err := (&postgres{db: fp}).Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if !fp.closed {
		t.Fatal("pool not closed")
	}
}

func TestLive_roundtrip(t *testing.T) {
	dsn := os.Getenv("SEARCH_PG_DSN")
	if dsn == "" {
		t.Skip("SEARCH_PG_DSN not set")
	}

	ctx := context.Background()

	s, err := New(search.Options{DSN: dsn})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	t.Cleanup(func() {
		_ = s.Close()
	})

	idx := "e2e-postgres"
	docs := []search.Document{
		{ID: "e2e-run-once", Index: idx, Content: "going for a run in the park", Metadata: map[string]any{"n": 1}},
		{ID: "e2e-run-many", Index: idx, Content: "run run run running fast every morning", Metadata: map[string]any{"n": 2}},
	}

	for _, d := range docs {
		if err = s.Index(ctx, d); err != nil {
			t.Fatalf("Index %s err = %v", d.ID, err)
		}
	}

	t.Cleanup(func() {
		for _, d := range docs {
			_ = s.Delete(ctx, d.ID)
		}
	})

	// Stemming: "running" matches the document containing only "run".
	res, err := s.Search(ctx, "running", search.QueryOptions{Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}

	// Rank ordering: the document repeating the term ranks first.
	if len(res.Hits) != 2 || res.Hits[0].ID != "e2e-run-many" {
		t.Fatalf("hits = %+v, want e2e-run-many first", res.Hits)
	}

	if err = s.Delete(ctx, "e2e-run-once"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	res, err = s.Search(ctx, "running", search.QueryOptions{Filters: map[string]string{"index": idx}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "e2e-run-many" {
		t.Fatalf("res after delete = %+v", res)
	}

	if err := s.Delete(ctx, "e2e-run-once"); err == nil {
		t.Fatal("second Delete err = nil, want NotFoundError")
	}
}

func TestNewPoolLazyNoConnect(t *testing.T) {
	t.Parallel()

	// pgxpool.New parses the DSN without connecting, so a closed port still
	// yields a usable pool value. Covers newPool's success path with no DB.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p, err := newPool(ctx, "postgres://127.0.0.1:1/db?sslmode=disable")
	if err != nil {
		t.Fatalf("newPool: %v", err)
	}

	if p == nil {
		t.Fatal("want non-nil pool")
	}

	p.Close()
}
