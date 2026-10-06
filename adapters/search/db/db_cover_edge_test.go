package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// stubSearchConn is a scripted coredb.DB double for postgres-leg and failure paths.
type stubSearchConn struct {
	coredb.DB
	dialectName string
	execErr     error
	execFn      func(ctx context.Context, query string, args ...any) (int64, error)
	queryFn     func(ctx context.Context, query string, args ...any) (coredb.Rows, error)
}

func (s *stubSearchConn) Dialect() string { return s.dialectName }

func (s *stubSearchConn) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if s.execFn != nil {
		return s.execFn(ctx, query, args...)
	}
	if s.execErr != nil {
		return 0, s.execErr
	}
	if s.DB != nil {
		return s.DB.Exec(ctx, query, args...)
	}
	return 0, nil
}

func (s *stubSearchConn) Query(ctx context.Context, query string, args ...any) (coredb.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, query, args...)
	}
	if s.DB != nil {
		return s.DB.Query(ctx, query, args...)
	}
	return nil, errors.New("postgres: no query script")
}

// stubSearchRows is a scripted coredb.Rows double.
type stubSearchRows struct {
	next []bool
	pos  int
	scan func(dest ...any) error
	err  error
}

func (r *stubSearchRows) Next() bool {
	if r.pos >= len(r.next) {
		return false
	}
	v := r.next[r.pos]
	r.pos++
	return v
}

func (r *stubSearchRows) Scan(dest ...any) error {
	if r.scan != nil {
		return r.scan(dest...)
	}
	return errors.New("postgres: no scan script")
}

func (r *stubSearchRows) Close() error { return nil }

func (r *stubSearchRows) Columns() ([]string, error) { return []string{"c"}, nil }

func (r *stubSearchRows) Err() error { return r.err }

// stubSearchStmt is a scripted coredb.Stmt double.
type stubSearchStmt struct {
	execErr error
	queryFn func(ctx context.Context, args ...any) (coredb.Rows, error)
}

func (s *stubSearchStmt) Query(ctx context.Context, args ...any) (coredb.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, args...)
	}
	return &stubSearchRows{}, nil
}

func (s *stubSearchStmt) Exec(_ context.Context, _ ...any) (int64, error) {
	if s.execErr != nil {
		return 0, s.execErr
	}
	return 1, nil
}

func (s *stubSearchStmt) Close() error { return nil }

// stubPreparerConn adds Prepare to stubSearchConn.
type stubPreparerConn struct {
	stubSearchConn
	prepareErr error
	stmt       *stubSearchStmt
}

func (s *stubPreparerConn) Prepare(_ context.Context, _ string) (coredb.Stmt, error) {
	if s.prepareErr != nil {
		return nil, s.prepareErr
	}
	if s.stmt != nil {
		return s.stmt, nil
	}
	return &stubSearchStmt{}, nil
}

// stubORMRow is a scripted orm.Row double.
type stubORMRow struct {
	scanFn func(dest ...any) error
}

func (r *stubORMRow) Scan(dest ...any) error { return r.scanFn(dest...) }

// stubSearchDialect is a minimal Dialect without full-text support.
type stubSearchDialect struct{}

func (stubSearchDialect) Name() string               { return "cover-noft" }
func (stubSearchDialect) Placeholder(_ int) string   { return "?" }
func (stubSearchDialect) QuoteIdent(s string) string { return `"` + s + `"` }

// stubNeitherFT is a FullTextDialect with neither flavor.
type stubNeitherFT struct{ stubSearchDialect }

func (stubNeitherFT) SupportsTSVector() bool { return false }
func (stubNeitherFT) SupportsFTS5() bool     { return false }

// mustSearchDriver returns a sqlite-backed *driver for fault injection.
func mustSearchDriver(t *testing.T) *driver {
	t.Helper()
	s := mustOpenMemory(t)
	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}
	return d
}

// TestCoverDocRowScan covers docRow.Scan branches.
func TestCoverDocRowScan(t *testing.T) {
	t.Parallel()
	var r docRow
	if err := r.Scan(&stubORMRow{scanFn: func(...any) error { return errors.New("scan boom") }}); err == nil {
		t.Fatal("Scan(scan fail) = nil, want error")
	}
	if err := r.Scan(&stubORMRow{scanFn: func(dest ...any) error {
		if p, ok := dest[3].(*any); ok {
			*p = 42
		}
		for i := 0; i < 3; i++ {
			if p, ok := dest[i].(*string); ok {
				*p = "x"
			}
		}
		return nil
	}}); err == nil {
		t.Fatal("Scan(bad meta) = nil, want error")
	}
	if err := r.Scan(&stubORMRow{scanFn: func(dest ...any) error {
		for i := 0; i < 3; i++ {
			if p, ok := dest[i].(*string); ok {
				*p = "v"
			}
		}
		if p, ok := dest[3].(*any); ok {
			*p = []byte(`{"content":"v"}`)
		}
		return nil
	}}); err != nil {
		t.Fatalf("Scan(ok) err = %v", err)
	}
	if r.ID != "v" || r.Index != "v" || r.Content != "v" {
		t.Fatalf("Scan(ok) = %+v", r)
	}
	_ = orm.NewTable[docRow]("t", docColumns)
}

// TestCoverCoerceMeta covers coerceMeta branches.
func TestCoverCoerceMeta(t *testing.T) {
	t.Parallel()
	if got, err := coerceMeta(nil); err != nil || got != nil {
		t.Fatalf("coerceMeta(nil) = %v,%v want nil,nil", got, err)
	}
	if got, err := coerceMeta([]byte("x")); err != nil || string(got) != "x" {
		t.Fatalf("coerceMeta(bytes) = %v,%v", got, err)
	}
	if got, err := coerceMeta("y"); err != nil || string(got) != "y" {
		t.Fatalf("coerceMeta(string) = %v,%v", got, err)
	}
	if _, err := coerceMeta(42); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("coerceMeta(42) err = %v, want ErrInvalidMetadata", err)
	}
}

// TestCoverDecodeHitMetaSearch covers decodeHitMeta branches.
func TestCoverDecodeHitMetaSearch(t *testing.T) {
	t.Parallel()
	if got, err := decodeHitMeta(nil); err != nil || got != nil {
		t.Fatalf("decodeHitMeta(nil) = %v,%v want nil,nil", got, err)
	}
	meta, err := decodeHitMeta([]byte(`{"a":1}`))
	if err != nil || meta["a"] != float64(1) {
		t.Fatalf("decodeHitMeta(bytes) = %v,%v", meta, err)
	}
	meta, err = decodeHitMeta("not-json")
	if err == nil {
		t.Fatalf("decodeHitMeta(bad) = %v, want error", meta)
	}
	if _, err := decodeHitMeta(42); !errors.Is(err, search.ErrInvalidMetadata) {
		t.Fatalf("decodeHitMeta(42) err = %v, want ErrInvalidMetadata", err)
	}
}

// TestCoverNewFromDBInvalidOptions covers Validate error with a live conn.
func TestCoverNewFromDBInvalidOptions(t *testing.T) {
	t.Parallel()
	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	if _, err := NewFromDB(conn, Options{Options: coredb.Options{MaxConns: -1}}); err == nil {
		t.Fatal("NewFromDB(bad opts) = nil, want error")
	}
}

// TestCoverCheckDialectSearch covers unknown, non-FT, and neither-flavor dialects.
func TestCoverCheckDialectSearch(t *testing.T) {
	t.Parallel()
	d := &driver{conn: &stubSearchConn{dialectName: "bogus-nope"}}
	if err := d.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect(bogus) err = %v, want unsupported", err)
	}
	if err := dialect.Register("cover-search-noft", func() dialect.Dialect { return stubSearchDialect{} }); err != nil {
		t.Fatalf("dialect.Register() err = %v", err)
	}
	d2 := &driver{conn: &stubSearchConn{dialectName: "cover-search-noft"}}
	if err := d2.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect(nonft) err = %v, want unsupported", err)
	}
	if err := dialect.Register("cover-search-neither", func() dialect.Dialect { return stubNeitherFT{} }); err != nil {
		t.Fatalf("dialect.Register() err = %v", err)
	}
	d3 := &driver{conn: &stubSearchConn{dialectName: "cover-search-neither"}}
	if err := d3.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect(neither) err = %v, want unsupported", err)
	}
}

// TestCoverEnsureSchemaSearch covers sqlite failure and postgres legs.
func TestCoverEnsureSchemaSearch(t *testing.T) {
	t.Parallel()
	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	s, err := NewFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("NewFromDB() err = %v", err)
	}
	dd, ok := s.(*driver)
	if !ok {
		t.Fatalf("NewFromDB() returned %T, want *driver", s)
	}
	dd.conn = &stubSearchConn{DB: conn, dialectName: "sqlite", execErr: errors.New("ddl boom")}
	if err := dd.ensureSchema(t.Context()); err == nil {
		t.Fatal("ensureSchema(sqlite fail) = nil, want error")
	}
	pgOK := &driver{conn: &stubSearchConn{dialectName: "postgres", execFn: func(context.Context, string, ...any) (int64, error) { return 1, nil }}}
	if err := pgOK.ensureSchema(t.Context()); err != nil {
		t.Fatalf("ensureSchema(pg ok) err = %v", err)
	}
	pgFail := &driver{conn: &stubSearchConn{dialectName: "postgres", execFn: func(context.Context, string, ...any) (int64, error) { return 0, errors.New("ddl boom") }}}
	if err := pgFail.ensureSchema(t.Context()); err == nil {
		t.Fatal("ensureSchema(pg fail) = nil, want error")
	}
}

// TestCoverUpsertExecFail covers upsertOne Exec failure.
func TestCoverUpsertExecFail(t *testing.T) {
	t.Parallel()
	d := mustSearchDriver(t)
	d.conn = &stubSearchConn{DB: d.conn, dialectName: "sqlite", execErr: errors.New("exec boom")}
	if err := d.upsertOne(t.Context(), "index", search.Document{ID: "x", Index: "i", Content: "c"}, []byte("{}")); err == nil {
		t.Fatal("upsertOne(exec fail) = nil, want error")
	}
}

// TestCoverUpsertBatchPostgres covers the postgres placeholder branch.
func TestCoverUpsertBatchPostgres(t *testing.T) {
	t.Parallel()
	d := &driver{conn: &stubSearchConn{dialectName: "postgres", execFn: func(_ context.Context, q string, args ...any) (int64, error) {
		if !strings.Contains(q, "$1") || !strings.Contains(q, "$4") {
			return 0, errors.New("want $n placeholders, got " + q)
		}
		if len(args) != 8 {
			return 0, errors.New("want 8 args")
		}
		return 2, nil
	}}}
	docs := []search.Document{
		{ID: "a", Index: "i", Content: "alpha"},
		{ID: "b", Index: "i", Content: "beta"},
	}
	metas := [][]byte{[]byte(`{"content":"alpha"}`), []byte(`{"content":"beta"}`)}
	if err := d.upsertBatch(t.Context(), docs, metas); err != nil {
		t.Fatalf("upsertBatch(pg) err = %v", err)
	}
}

// TestCoverExecBatch covers Preparer and fallback branches.
func TestCoverExecBatch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	okPrep := &driver{conn: &stubPreparerConn{stubSearchConn: stubSearchConn{dialectName: "sqlite"}, stmt: &stubSearchStmt{}}}
	if err := okPrep.execBatch(ctx, "q", []any{"a"}); err != nil {
		t.Fatalf("execBatch(prep ok) err = %v", err)
	}
	failPrep := &driver{conn: &stubPreparerConn{stubSearchConn: stubSearchConn{dialectName: "sqlite"}, prepareErr: errors.New("prep boom")}}
	if err := failPrep.execBatch(ctx, "q", []any{"a"}); err == nil {
		t.Fatal("execBatch(prep fail) = nil, want error")
	}
	failStmt := &driver{conn: &stubPreparerConn{stubSearchConn: stubSearchConn{dialectName: "sqlite"}, stmt: &stubSearchStmt{execErr: errors.New("stmt boom")}}}
	if err := failStmt.execBatch(ctx, "q", []any{"a"}); err == nil {
		t.Fatal("execBatch(stmt fail) = nil, want error")
	}
	failFall := &driver{conn: &stubSearchConn{dialectName: "sqlite", execErr: errors.New("exec boom")}}
	if err := failFall.execBatch(ctx, "q", []any{"a"}); err == nil {
		t.Fatal("execBatch(fallback fail) = nil, want error")
	}
}

// TestCoverSearchPostgres covers searchPostgres branches.
func TestCoverSearchPostgres(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	qFail := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return nil, errors.New("hits boom")
	}}}
	if _, err := qFail.searchPostgres(ctx, "q", nil, 10, 0); err == nil {
		t.Fatal("searchPostgres(hits fail) = nil, want error")
	}
	scanFail := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(_ context.Context, q string, _ ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*int64); ok {
					*p = 1
				}
				return nil
			}}, nil
		}
		return &stubSearchRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
	}}}
	if _, err := scanFail.searchPostgres(ctx, "q", nil, 10, 0); err == nil {
		t.Fatal("searchPostgres(scan fail) = nil, want error")
	}
	metaFail := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(_ context.Context, q string, _ ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*int64); ok {
					*p = 1
				}
				return nil
			}}, nil
		}
		return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
			if p, ok := dest[0].(*string); ok {
				*p = "a"
			}
			if p, ok := dest[1].(*any); ok {
				*p = 42
			}
			if p, ok := dest[2].(*float64); ok {
				*p = 1.0
			}
			return nil
		}}, nil
	}}}
	if _, err := metaFail.searchPostgres(ctx, "q", nil, 10, 0); err == nil {
		t.Fatal("searchPostgres(meta fail) = nil, want error")
	}
	rowsErr := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(_ context.Context, q string, _ ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*int64); ok {
					*p = 1
				}
				return nil
			}}, nil
		}
		return &stubSearchRows{next: []bool{}, err: errors.New("iter boom")}, nil
	}}}
	if _, err := rowsErr.searchPostgres(ctx, "q", nil, 10, 0); err == nil {
		t.Fatal("searchPostgres(rows err) = nil, want error")
	}
	countFail := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(_ context.Context, q string, _ ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return nil, errors.New("count boom")
		}
		return &stubSearchRows{next: []bool{}}, nil
	}}}
	if _, err := countFail.searchPostgres(ctx, "q", nil, 10, 0); err == nil {
		t.Fatal("searchPostgres(count fail) = nil, want error")
	}
	ok := &driver{conn: &stubSearchConn{dialectName: "postgres", queryFn: func(_ context.Context, q string, _ ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*int64); ok {
					*p = 1
				}
				return nil
			}}, nil
		}
		hit := false
		return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
			if hit {
				return errors.New("no more")
			}
			hit = true
			if p, ok := dest[0].(*string); ok {
				*p = "a"
			}
			if p, ok := dest[1].(*any); ok {
				*p = []byte(`{"content":"x"}`)
			}
			if p, ok := dest[2].(*float64); ok {
				*p = 0.7
			}
			return nil
		}}, nil
	}}}
	res, err := ok.searchPostgres(ctx, "q", nil, 10, 0)
	if err != nil {
		t.Fatalf("searchPostgres(ok) err = %v", err)
	}
	if res.Total != 1 || len(res.Hits) != 1 || res.Hits[0].ID != "a" {
		t.Fatalf("searchPostgres(ok) = %+v, want hit a", res)
	}
	if _, err := ok.Search(ctx, "q", search.QueryOptions{Limit: 10}); err != nil {
		t.Fatalf("Search(pg dispatch) err = %v", err)
	}
}

// TestCoverSearchSQLiteErrors covers searchSQLite failure legs.
func TestCoverSearchSQLiteErrors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	qFail := mustSearchDriver(t)
	qFail.conn = &stubSearchConn{DB: qFail.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return nil, errors.New("hits boom")
	}}
	if _, err := qFail.searchSQLite(ctx, "hello", nil, 10, 0); err == nil {
		t.Fatal("searchSQLite(q fail) = nil, want error")
	}
	scanFail := mustSearchDriver(t)
	backing := scanFail.conn
	scanFail.conn = &stubSearchConn{DB: backing, dialectName: "sqlite", queryFn: func(_ context.Context, q string, args ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return backing.Query(ctx, q, args...)
		}
		return &stubSearchRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
	}}
	// Seed one doc so match is non-empty.
	if err := func() error {
		scanFail.conn = backing
		defer func() {
			scanFail.conn = &stubSearchConn{DB: backing, dialectName: "sqlite", queryFn: func(_ context.Context, q string, args ...any) (coredb.Rows, error) {
				if strings.Contains(q, "COUNT") {
					return backing.Query(ctx, q, args...)
				}
				return &stubSearchRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
			}}
		}()
		return scanFail.Index(ctx, search.Document{ID: "s1", Index: "i", Content: "scanfail probe content"})
	}(); err != nil {
		t.Fatalf("seed Index() err = %v", err)
	}
	if _, err := scanFail.searchSQLite(ctx, "scanfail", map[string]string{"index": "i"}, 10, 0); err == nil {
		t.Fatal("searchSQLite(scan fail) = nil, want error")
	}
	rowsErr := mustSearchDriver(t)
	backing2 := rowsErr.conn
	if err := rowsErr.Index(ctx, search.Document{ID: "r1", Index: "i", Content: "rowserr probe content"}); err != nil {
		t.Fatalf("seed Index() err = %v", err)
	}
	rowsErr.conn = &stubSearchConn{DB: backing2, dialectName: "sqlite", queryFn: func(_ context.Context, q string, args ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return backing2.Query(ctx, q, args...)
		}
		return &stubSearchRows{next: []bool{}, err: errors.New("iter boom")}, nil
	}}
	if _, err := rowsErr.searchSQLite(ctx, "rowserr", map[string]string{"index": "i"}, 10, 0); err == nil {
		t.Fatal("searchSQLite(rows err) = nil, want error")
	}
	metaFail := mustSearchDriver(t)
	backing3 := metaFail.conn
	if err := metaFail.Index(ctx, search.Document{ID: "m1", Index: "i", Content: "metafail probe content"}); err != nil {
		t.Fatalf("seed Index() err = %v", err)
	}
	metaFail.conn = &stubSearchConn{DB: backing3, dialectName: "sqlite", queryFn: func(_ context.Context, q string, args ...any) (coredb.Rows, error) {
		if strings.Contains(q, "COUNT") {
			return backing3.Query(ctx, q, args...)
		}
		return &stubSearchRows{next: []bool{true}, scan: func(dest ...any) error {
			if p, ok := dest[0].(*string); ok {
				*p = "m1"
			}
			if p, ok := dest[1].(*any); ok {
				*p = 42
			}
			if p, ok := dest[2].(*float64); ok {
				*p = 1.0
			}
			return nil
		}}, nil
	}}
	if _, err := metaFail.searchSQLite(ctx, "metafail", map[string]string{"index": "i"}, 10, 0); err == nil {
		t.Fatal("searchSQLite(meta fail) = nil, want error")
	}
}

// TestCoverSearchCount covers searchCount branches.
func TestCoverSearchCount(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	d := mustSearchDriver(t)
	d.conn = &stubSearchConn{DB: d.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return nil, errors.New("count boom")
	}}
	if _, err := d.searchCount(ctx, "SELECT COUNT(*)", nil); err == nil {
		t.Fatal("searchCount(q fail) = nil, want error")
	}
	d.conn = &stubSearchConn{DB: d.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubSearchRows{next: []bool{}, err: errors.New("iter boom")}, nil
	}}
	if _, err := d.searchCount(ctx, "SELECT COUNT(*)", nil); err == nil {
		t.Fatal("searchCount(iter err) = nil, want error")
	}
	d.conn = &stubSearchConn{DB: d.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubSearchRows{next: []bool{}}, nil
	}}
	if _, err := d.searchCount(ctx, "SELECT COUNT(*)", nil); !errors.Is(err, search.ErrNotFound) {
		t.Fatalf("searchCount(no rows) err = %v, want ErrNotFound", err)
	}
	d.conn = &stubSearchConn{DB: d.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubSearchRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
	}}
	if _, err := d.searchCount(ctx, "SELECT COUNT(*)", nil); err == nil {
		t.Fatal("searchCount(scan fail) = nil, want error")
	}
}

// TestCoverRegisterShared covers the shared-factory registration.
func TestCoverRegisterShared(t *testing.T) {
	Register()
	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	for _, a := range []search.Adapter{search.DB, search.Postgres, search.SQLite} {
		s, err := search.OpenShared(a, conn, search.Options{})
		if err != nil {
			t.Fatalf("OpenShared(%s) err = %v", a, err)
		}
		_ = s.Close()
	}
}
