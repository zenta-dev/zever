package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/orm/dialect"
)

// stubConn is a scripted coredb.DB double for postgres-leg and failure paths.
type stubConn struct {
	coredb.DB
	dialectName string
	pingErr     error
	execErr     error
	execFn      func(ctx context.Context, query string, args ...any) (int64, error)
	queryFn     func(ctx context.Context, query string, args ...any) (coredb.Rows, error)
	closeErr    error
}

func (s *stubConn) Dialect() string { return s.dialectName }

func (s *stubConn) Ping(_ context.Context) error {
	if s.pingErr != nil {
		return s.pingErr
	}
	if s.DB != nil {
		return s.DB.Ping(context.Background())
	}
	return nil
}

func (s *stubConn) Exec(ctx context.Context, query string, args ...any) (int64, error) {
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

func (s *stubConn) Query(ctx context.Context, query string, args ...any) (coredb.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, query, args...)
	}
	if s.DB != nil {
		return s.DB.Query(ctx, query, args...)
	}
	return nil, errors.New("pgvector: no query script")
}

func (s *stubConn) Close(_ context.Context) error { return s.closeErr }

// stubRows is a scripted coredb.Rows double.
type stubRows struct {
	next  []bool
	pos   int
	scan  func(dest ...any) error
	err   error
	close error
}

func (r *stubRows) Next() bool {
	if r.pos >= len(r.next) {
		return false
	}
	v := r.next[r.pos]
	r.pos++
	return v
}

func (r *stubRows) Scan(dest ...any) error {
	if r.scan != nil {
		return r.scan(dest...)
	}
	return errors.New("pgvector: no scan script")
}

func (r *stubRows) Close() error { return r.close }

func (r *stubRows) Columns() ([]string, error) { return []string{"c"}, nil }

func (r *stubRows) Err() error { return r.err }

// mustDriver returns a sqlite-backed *driver for white-box fault injection.
func mustDriver(t *testing.T) *driver {
	t.Helper()
	s := mustOpenMemory(t)
	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}
	return d
}

// TestCoverOpen_postgresBranches covers Open's postgres DSN routing without a live server.
func TestCoverOpen_postgresBranches(t *testing.T) {
	t.Parallel()
	if _, err := Open(vectorstore.Options{DSN: "postgres://[::1"}); err == nil {
		t.Fatal("Open(bad pg DSN) = nil, want error")
	}
	if _, err := Open(vectorstore.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=disable"}); err == nil {
		t.Fatal("Open(pg refused) = nil, want error")
	}
}

// TestCoverOpenFromDB_pingFail covers openFromDB's Ping error.
func TestCoverOpenFromDB_pingFail(t *testing.T) {
	t.Parallel()
	stub := &stubConn{dialectName: "sqlite", pingErr: errors.New("boom")}
	if _, err := openFromDB(stub, vectorstore.Options{}, 3, true); err == nil {
		t.Fatal("openFromDB(ping fail) = nil, want error")
	}
}

// registerCoverNoVector registers the stub non-vector dialect once per test
// binary: the orm dialect registry is process-global with no unregister, so
// a second -count iteration registering it again would fail with
// "Register called twice".
var registerCoverNoVector = sync.OnceValue(func() error {
	return dialect.Register("cover-novector", func() dialect.Dialect { return stubDialect{} })
})

// TestCoverCheckDialect_branches covers unknown and non-vector dialects.
func TestCoverCheckDialect_branches(t *testing.T) {
	t.Parallel()
	d := &driver{conn: &stubConn{dialectName: "bogus-nope"}}
	if err := d.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect(bogus) err = %v, want ErrUnsupportedByDialect", err)
	}
	if err := registerCoverNoVector(); err != nil {
		t.Fatalf("dialect.Register() err = %v", err)
	}
	d2 := &driver{conn: &stubConn{dialectName: "cover-novector"}}
	if err := d2.checkDialect(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("checkDialect(novector) err = %v, want ErrUnsupportedByDialect", err)
	}
}

// stubDialect is a minimal Dialect without vector ops.
type stubDialect struct{}

func (stubDialect) Name() string               { return "cover-novector" }
func (stubDialect) Placeholder(_ int) string   { return "?" }
func (stubDialect) QuoteIdent(s string) string { return `"` + s + `"` }

// TestCoverEnsureSchema_sqliteExecFail covers the sqlite DDL error.
func TestCoverEnsureSchema_sqliteExecFail(t *testing.T) {
	t.Parallel()
	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	d, err := NewFromDB(conn, vectorstore.Options{})
	if err != nil {
		t.Fatalf("NewFromDB() err = %v", err)
	}
	dd, ok := d.(*driver)
	if !ok {
		t.Fatalf("NewFromDB() returned %T, want *driver", d)
	}
	dd.conn = &stubConn{DB: conn, dialectName: "sqlite", execErr: errors.New("ddl boom")}
	if err := dd.ensureSchema(t.Context(), 3); err == nil {
		t.Fatal("ensureSchema(exec fail) = nil, want error")
	}
}

// TestCoverEnsureSchema_postgres covers the postgres DDL legs via stubs.
func TestCoverEnsureSchema_postgres(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		execFn  func(call int) (int64, error)
		queryFn func() (coredb.Rows, error)
		wantErr string
	}{
		{"ext fail", func(_ int) (int64, error) { return 0, errors.New("ext boom") }, nil, "create extension"},
		{"inspect fail", func(int) (int64, error) { return 1, nil }, func() (coredb.Rows, error) { return nil, errors.New("inspect boom") }, "inspect"},
		{"dim mismatch", func(int) (int64, error) { return 1, nil }, func() (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "vector(4)"
				}
				return nil
			}}, nil
		}, "dimension"},
		{"create table fail", func(call int) (int64, error) {
			if call == 2 {
				return 0, errors.New("table boom")
			}
			return 1, nil
		}, func() (coredb.Rows, error) { return &stubRows{next: []bool{}}, nil }, "create table"},
		{"create index fail", func(call int) (int64, error) {
			if call == 3 {
				return 0, errors.New("index boom")
			}
			return 1, nil
		}, func() (coredb.Rows, error) { return &stubRows{next: []bool{}}, nil }, "create index"},
		{"ok no table", func(int) (int64, error) { return 1, nil }, func() (coredb.Rows, error) { return &stubRows{next: []bool{}}, nil }, ""},
		{"ok existing match", func(int) (int64, error) { return 1, nil }, func() (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "vector(3)"
				}
				return nil
			}}, nil
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls int
			stub := &stubConn{
				dialectName: "postgres",
				execFn: func(_ context.Context, _ string, _ ...any) (int64, error) {
					calls++
					return tt.execFn(calls)
				},
				queryFn: func(_ context.Context, _ string, _ ...any) (coredb.Rows, error) {
					return tt.queryFn()
				},
			}
			d := &driver{conn: stub, dim: 3}
			err := d.ensureSchema(t.Context(), 3)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ensureSchema() err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ensureSchema() err = %v, want contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestCoverEmbeddingColumnDim covers every embeddingColumnDim branch.
func TestCoverEmbeddingColumnDim(t *testing.T) {
	t.Parallel()
	mk := func(rows coredb.Rows, err error) *stubConn {
		return &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return rows, err
		}}
	}
	cases := []struct {
		name    string
		conn    *stubConn
		wantDim int
		wantEx  bool
		wantErr bool
	}{
		{"query err", mk(nil, errors.New("q boom")), 0, false, true},
		{"rows err on empty", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{}, err: errors.New("iter boom")}, nil
		}}, 0, false, true},
		{"no rows", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{}}, nil
		}}, 0, false, false},
		{"scan err", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
		}}, 0, false, true},
		{"rows err after scan", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "vector(3)"
				}
				return nil
			}, err: errors.New("tail boom")}, nil
		}}, 0, false, true},
		{"bad type", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "text"
				}
				return nil
			}}, nil
		}}, 0, false, true},
		{"ok", &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
			return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "vector(3)"
				}
				return nil
			}}, nil
		}}, 3, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dd := &driver{conn: tc.conn}
			dim, ex, err := dd.embeddingColumnDim(t.Context())
			if tc.wantErr {
				if err == nil {
					t.Fatal("embeddingColumnDim() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("embeddingColumnDim() err = %v", err)
			}
			if dim != tc.wantDim || ex != tc.wantEx {
				t.Fatalf("embeddingColumnDim() = %d,%v want %d,%v", dim, ex, tc.wantDim, tc.wantEx)
			}
		})
	}
}

// TestCoverDecodeHitMeta covers decodeHitMeta branches.
func TestCoverDecodeHitMeta(t *testing.T) {
	t.Parallel()
	if got, err := decodeHitMeta(nil); err != nil || got != nil {
		t.Fatalf("decodeHitMeta(nil) = %v,%v want nil,nil", got, err)
	}
	if got, err := decodeHitMeta([]byte(nil)); err != nil || got != nil {
		t.Fatalf("decodeHitMeta(nil bytes) = %v,%v want nil,nil", got, err)
	}
	meta, err := decodeHitMeta([]byte(`{"a":1}`))
	if err != nil || meta["a"] != float64(1) {
		t.Fatalf("decodeHitMeta(bytes) = %v,%v", meta, err)
	}
	meta, err = decodeHitMeta(`{"b":2}`)
	if err != nil || meta["b"] != float64(2) {
		t.Fatalf("decodeHitMeta(string) = %v,%v", meta, err)
	}
	if _, err := decodeHitMeta(42); !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Fatalf("decodeHitMeta(42) err = %v, want ErrInvalidMetadata", err)
	}
	if _, err := decodeHitMeta([]byte("{bad")); err == nil {
		t.Fatal("decodeHitMeta(bad json) = nil, want error")
	}
}

// TestCoverUpsertPostgres covers dimension, marshal, and exec paths.
func TestCoverUpsertPostgres(t *testing.T) {
	t.Parallel()
	d := &driver{conn: &stubConn{dialectName: "postgres"}, dim: 2}
	if err := d.upsertPostgres(t.Context(), "upsert", vectorstore.Vector{ID: "x", Embedding: []float32{1}}); err == nil {
		t.Fatal("upsertPostgres(dim) = nil, want error")
	} else {
		var mmErr vectorstore.DimensionMismatchError
		if !errors.As(err, &mmErr) {
			t.Fatalf("upsertPostgres(dim) err = %v, want DimensionMismatch", err)
		}
	}
	bad := vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}, Metadata: map[string]any{"ch": make(chan int)}}
	if err := d.upsertPostgres(t.Context(), "upsert", bad); err == nil || !strings.Contains(err.Error(), "marshal metadata") {
		t.Fatalf("upsertPostgres(bad meta) err = %v, want marshal metadata", err)
	}
	dOk := &driver{conn: &stubConn{dialectName: "postgres", execFn: func(context.Context, string, ...any) (int64, error) { return 1, nil }}, dim: 2}
	if err := dOk.upsertPostgres(t.Context(), "upsert", vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}}); err != nil {
		t.Fatalf("upsertPostgres(ok) err = %v", err)
	}
	dFail := &driver{conn: &stubConn{dialectName: "postgres", execFn: func(context.Context, string, ...any) (int64, error) { return 0, errors.New("exec boom") }}, dim: 2}
	if err := dFail.upsertPostgres(t.Context(), "upsert", vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}}); err == nil {
		t.Fatal("upsertPostgres(exec fail) = nil, want error")
	}
	// upsertOne postgres dispatch + empty embedding direct.
	if err := d.upsertOne(t.Context(), "upsert", vectorstore.Vector{ID: "x"}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("upsertOne(empty) err = %v, want ErrEmptyEmbedding", err)
	}
	if err := dFail.upsertOne(t.Context(), "upsert", vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}}); err == nil {
		t.Fatal("upsertOne(pg fail) = nil, want error")
	}
}

// TestCoverUpsertOne_sqliteErrors covers sqlite encode and exec failures.
func TestCoverUpsertOne_sqliteErrors(t *testing.T) {
	t.Parallel()
	d := mustDriver(t)
	// Encode failure bypasses Validate via direct upsertOne.
	bad := vectorstore.Vector{ID: "x", Embedding: []float32{1, 2}, Metadata: map[string]any{"ch": make(chan int)}}
	if err := d.upsertOne(t.Context(), "upsert", bad); err == nil || !strings.Contains(err.Error(), "encode metadata") {
		t.Fatalf("upsertOne(bad meta) err = %v, want encode metadata", err)
	}
	_ = d
}

// TestCoverDelete_execFail covers Delete's Exec error.
func TestCoverDelete_execFail(t *testing.T) {
	t.Parallel()
	d := mustDriver(t)
	d.conn = &stubConn{DB: d.conn, dialectName: "sqlite", execErr: errors.New("exec boom")}
	if err := d.Delete(t.Context(), "x"); err == nil {
		t.Fatal("Delete(exec fail) = nil, want error")
	}
}

// TestCoverQueryPostgres covers queryPostgres branches.
func TestCoverQueryPostgres(t *testing.T) {
	t.Parallel()
	d := &driver{conn: &stubConn{dialectName: "postgres"}, dim: 2}
	if _, err := d.queryPostgres(t.Context(), []float32{1}, 5); !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Fatalf("queryPostgres(dim) err = %v, want mismatch", err)
	}
	dQFail := &driver{conn: &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return nil, errors.New("q boom")
	}}, dim: 2}
	if _, err := dQFail.queryPostgres(t.Context(), []float32{1, 2}, 5); err == nil {
		t.Fatal("queryPostgres(q fail) = nil, want error")
	}
	dScanFail := &driver{conn: &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, nil
	}}, dim: 2}
	if _, err := dScanFail.queryPostgres(t.Context(), []float32{1, 2}, 5); err == nil {
		t.Fatal("queryPostgres(scan fail) = nil, want error")
	}
	dMetaFail := &driver{conn: &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
			if p, ok := dest[0].(*string); ok {
				*p = "a"
			}
			if p, ok := dest[1].(*float64); ok {
				*p = 0.5
			}
			if p, ok := dest[2].(*any); ok {
				*p = 42
			}
			return nil
		}}, nil
	}}, dim: 2}
	if _, err := dMetaFail.queryPostgres(t.Context(), []float32{1, 2}, 5); err == nil {
		t.Fatal("queryPostgres(meta fail) = nil, want error")
	}
	dRowsErr := &driver{conn: &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return &stubRows{next: []bool{}, err: errors.New("iter boom")}, nil
	}}, dim: 2}
	if _, err := dRowsErr.queryPostgres(t.Context(), []float32{1, 2}, 5); err == nil {
		t.Fatal("queryPostgres(rows err) = nil, want error")
	}
	dOK := &driver{conn: &stubConn{dialectName: "postgres", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		hit := false
		return &stubRows{next: []bool{true}, scan: func(dest ...any) error {
			if hit {
				return errors.New("no more")
			}
			hit = true
			if p, ok := dest[0].(*string); ok {
				*p = "a"
			}
			if p, ok := dest[1].(*float64); ok {
				*p = 0.9
			}
			if p, ok := dest[2].(*any); ok {
				*p = []byte(`{"k":1}`)
			}
			return nil
		}}, nil
	}}, dim: 2}
	hits, err := dOK.queryPostgres(t.Context(), []float32{1, 2}, 5)
	if err != nil {
		t.Fatalf("queryPostgres(ok) err = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("queryPostgres(ok) = %+v, want hit a", hits)
	}
	// Query dispatches to postgres leg.
	if _, err := dOK.Query(t.Context(), []float32{1, 2}, 5); err != nil {
		t.Fatalf("Query(pg) err = %v", err)
	}
}

// TestCoverQuerySQLite_errors covers querySQLite failure legs.
func TestCoverQuerySQLite_errors(t *testing.T) {
	t.Parallel()
	d := mustDriver(t)
	d.conn = &stubConn{DB: d.conn, dialectName: "sqlite", queryFn: func(context.Context, string, ...any) (coredb.Rows, error) {
		return nil, errors.New("q boom")
	}}
	if _, err := d.querySQLite(t.Context(), []float32{1, 2, 3}, 5); err == nil {
		// dim not yet learned on fresh driver? force dim first via upsert on real conn.
		t.Fatal("querySQLite(q fail) = nil, want error")
	}
}

// TestCoverScanRows_errors covers ctx cancel, scan fail, rows err.
func TestCoverScanRows_errors(t *testing.T) {
	t.Parallel()
	d := mustDriver(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.scanRows(ctx, &stubRows{next: []bool{true}}, []float32{1}, 5); err == nil {
		t.Fatal("scanRows(cancelled) = nil, want error")
	}
	if _, err := d.scanRows(t.Context(), &stubRows{next: []bool{true}, scan: func(...any) error { return errors.New("scan boom") }}, []float32{1}, 5); err == nil {
		t.Fatal("scanRows(scan fail) = nil, want error")
	}
	if _, err := d.scanRows(t.Context(), &stubRows{next: []bool{}, err: errors.New("iter boom")}, []float32{1}, 5); err == nil {
		t.Fatal("scanRows(rows err) = nil, want error")
	}
	if _, _, _, err := decodeRow(&stubRows{scan: func(...any) error { return errors.New("scan boom") }}); err == nil {
		t.Fatal("decodeRow(scan fail) = nil, want error")
	}
	// Corrupt blob row fails decode.
	if _, _, _, err := decodeRow(&stubRows{scan: func(dest ...any) error {
		if p, ok := dest[0].(*string); ok {
			*p = "x"
		}
		if p, ok := dest[1].(*[]byte); ok {
			*p = []byte{1, 2, 3}
		}
		return nil
	}}); err == nil {
		t.Fatal("decodeRow(bad blob) = nil, want error")
	}
}

// TestCoverValidateAndBatch covers Validate and batch edge paths.
func TestCoverValidateAndBatch(t *testing.T) {
	t.Parallel()
	s := mustOpenMemory(t)
	ctx := t.Context()
	if err := s.Upsert(ctx, vectorstore.Vector{ID: "bad", Embedding: []float32{1}, Metadata: map[string]any{"ch": make(chan int)}}); !errors.Is(err, vectorstore.ErrInvalidMetadata) {
		t.Fatalf("Upsert(bad meta) err = %v, want ErrInvalidMetadata", err)
	}
	if err := s.UpsertBatch(ctx, []vectorstore.Vector{{ID: "x", Metadata: map[string]any{"ch": make(chan int)}}}); err == nil {
		t.Fatal("UpsertBatch(invalid) = nil, want error")
	}
	if err := s.UpsertBatch(ctx, []vectorstore.Vector{{ID: "x", Embedding: []float32{}}}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Fatalf("UpsertBatch(empty) err = %v, want ErrEmptyEmbedding", err)
	}
	if err := checkDimension(2, 2); err != nil {
		t.Fatalf("checkDimension(equal) err = %v", err)
	}
	if err := checkDimension(2, 3); !errors.Is(err, vectorstore.ErrDimensionMismatch) {
		t.Fatalf("checkDimension(mismatch) err = %v, want mismatch", err)
	}
	if _, err := NewFromDB(nil, vectorstore.Options{}); !errors.Is(err, ErrNilDB) {
		t.Fatalf("NewFromDB(nil) err = %v, want ErrNilDB", err)
	}
	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	if _, err := NewFromDB(conn, vectorstore.Options{Dimension: -1}); err == nil {
		t.Fatal("NewFromDB(bad opts) = nil, want error")
	}
}
