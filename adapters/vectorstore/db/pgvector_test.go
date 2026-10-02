package db

import (
	"errors"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// stubDB is a coredb.DB double with a scripted dialect.
type stubDB struct {
	coredb.DB
	dialect string
}

func (s *stubDB) Dialect() string { return s.dialect }

func mustOpenMemory(t *testing.T) vectorstore.VectorStore {
	t.Helper()

	s, err := New(vectorstore.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func mustUpsert(t *testing.T, s vectorstore.VectorStore, vec vectorstore.Vector) {
	t.Helper()

	if err := s.Upsert(t.Context(), vec); err != nil {
		t.Fatalf("Upsert(%q) error = %v", vec.ID, err)
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
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := New(vectorstore.Options{Dimension: -1}); err == nil {
		t.Fatal("New(negative dimension) = nil, want error")
	}
}

func TestNewFromDB_nil(t *testing.T) {
	t.Parallel()

	if _, err := NewFromDB(nil, vectorstore.Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

func TestNewFromDB_borrowsConnection(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	s, err := NewFromDB(conn, vectorstore.Options{})
	if err != nil {
		t.Fatalf("NewFromDB() error = %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("NewFromDB() returned %T, want *driver", s)
	}
	if d.owns {
		t.Fatal("owns = true, want false for borrowed connection")
	}

	mustUpsert(t, s, vectorstore.Vector{ID: "borrowed", Embedding: []float32{1, 0, 0}})

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Borrowed Close is a no-op: the caller's connection stays usable.
	if err := conn.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() after borrowed Close error = %v, want usable conn", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

func TestOpenFromDB_unsupportedDialect(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	stub := &stubDB{DB: conn, dialect: "mysql"}

	if _, err := OpenFromDB(stub, vectorstore.Options{}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("OpenFromDB(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}

	// Failed OpenFromDB never closes the caller's connection.
	if err := conn.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() after failed OpenFromDB error = %v, want usable conn", err)
	}
}

func TestDriver_unsupportedDialectOps(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	s, err := NewFromDB(conn, vectorstore.Options{})
	if err != nil {
		t.Fatalf("NewFromDB() error = %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("NewFromDB() returned %T, want *driver", s)
	}
	d.conn = &stubDB{DB: conn, dialect: "mysql"}

	ctx := t.Context()

	if err := d.Upsert(ctx, vectorstore.Vector{ID: "x", Embedding: []float32{1}}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Upsert(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}

	if err := d.UpsertBatch(ctx, []vectorstore.Vector{{ID: "x", Embedding: []float32{1}}}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("UpsertBatch(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}

	if err := d.Delete(ctx, "x"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Delete(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}

	if _, err := d.Query(ctx, []float32{1}, 5); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Query(mysql) err = %v, want ErrUnsupportedByDialect", err)
	}
}

func TestSQLiteLeg_emptyEmbedding(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	if err := s.Upsert(ctx, vectorstore.Vector{ID: "ee-one"}); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Upsert(nil embedding) err = %v, want ErrEmptyEmbedding", err)
	}

	if _, err := s.Query(ctx, nil, 5); !errors.Is(err, vectorstore.ErrEmptyEmbedding) {
		t.Errorf("Query(nil embedding) err = %v, want ErrEmptyEmbedding", err)
	}
}

func TestSQLiteLeg_dimensionMismatch(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)
	ctx := t.Context()

	mustUpsert(t, s, vectorstore.Vector{ID: "dm-one", Embedding: []float32{1, 0, 0}})

	err := s.Upsert(ctx, vectorstore.Vector{ID: "dm-two", Embedding: []float32{1, 0}})

	var mmErr *vectorstore.DimensionMismatchError
	if !errors.As(err, &mmErr) {
		t.Fatalf("Upsert(wrong dim) err = %T %v, want *DimensionMismatchError", err, err)
	}

	if mmErr.Got != 2 || mmErr.Want != 3 {
		t.Errorf("DimensionMismatchError = got %d want %d, want got 2 want 3", mmErr.Got, mmErr.Want)
	}
}

func TestSQLiteLeg_deleteMissing(t *testing.T) {
	t.Parallel()

	s := mustOpenMemory(t)

	err := s.Delete(t.Context(), "no-such-vector")

	var nfErr *vectorstore.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("Delete(missing) err = %T %v, want *NotFoundError", err, err)
	}

	if !errors.Is(err, vectorstore.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
}

func TestCheckDimension(t *testing.T) {
	t.Parallel()

	if err := checkDimension(3, 3); err != nil {
		t.Fatalf("checkDimension(3, 3) error = %v, want nil", err)
	}

	err := checkDimension(3, 5)
	if err == nil || !strings.Contains(err.Error(), "3") || !strings.Contains(err.Error(), "5") {
		t.Fatalf("checkDimension(3, 5) err = %v, want both numbers", err)
	}
}

func TestParseVectorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ     string
		want    int
		wantErr bool
	}{
		{"vector(3)", 3, false},
		{"vector(1536)", 1536, false},
		{"text", 0, true},
		{"vector(0)", 0, true},
		{"vector(-1)", 0, true},
		{"vector(abc)", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			t.Parallel()

			got, err := parseVectorType(tt.typ)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseVectorType() = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("parseVectorType() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("parseVectorType() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEmbeddingCodec_roundTrip(t *testing.T) {
	t.Parallel()

	blob := encodeEmbedding([]float32{1, 0.5, -2})

	got, err := decodeEmbedding(blob)
	if err != nil {
		t.Fatalf("decodeEmbedding() error = %v", err)
	}

	if len(got) != 3 || got[0] != 1 || got[1] != 0.5 || got[2] != -2 {
		t.Fatalf("decodeEmbedding() = %v, want [1 0.5 -2]", got)
	}

	// Rows written by the old sqlite adapter decode too.
	legacy, err := embeddingCodec.Encode([]float32{1, 0})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	got, err = decodeEmbedding(legacy)
	if err != nil {
		t.Fatalf("decodeEmbedding(legacy JSON) error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("decodeEmbedding(legacy JSON) = %v, want 2 dims", got)
	}

	if _, err := decodeEmbedding(nil); err == nil {
		t.Fatal("decodeEmbedding(nil) = nil, want error")
	}

	if _, err := decodeEmbedding([]byte{1, 2, 3}); err == nil {
		t.Fatal("decodeEmbedding(3 bytes) = nil, want error")
	}
}

// TestOpenRegister_allNames proves the canonical "db" name and both legacy
// aliases resolve through the registry to a working store.
func TestOpenRegister_allNames(t *testing.T) {
	Register()

	for _, adapter := range []vectorstore.Adapter{vectorstore.DB, vectorstore.PGVector, vectorstore.SQLite} {
		s, err := vectorstore.Open(adapter, vectorstore.Options{})
		if err != nil {
			t.Fatalf("Open(%s) error = %v", adapter, err)
		}

		if err := s.Upsert(t.Context(), vectorstore.Vector{ID: "w", Embedding: []float32{1, 0}}); err != nil {
			t.Errorf("Open(%s) Upsert() error = %v", adapter, err)
		}

		_ = s.Close()
	}
}
