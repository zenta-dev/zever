package pgvector

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zenta-dev/zever/core/vectorstore"
)

var _ pgx.Rows = (*fakeRows)(nil)

// fakeRows is an in-memory pgx.Rows for unit tests without a live database.
type fakeRows struct {
	rows    [][]any
	i       int
	err     error
	scanErr error
	closed  bool
}

// Close marks the rows closed. pgx closes idempotently with no return.
func (r *fakeRows) Close() { r.closed = true }

// Err returns the injected iteration error.
func (r *fakeRows) Err() error { return r.err }

// CommandTag returns zero tag; fakes never execute commands.
func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

// FieldDescriptions returns nil; fakes carry no column metadata.
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

// TypeMap returns a fresh type map; the adapter never consults it in tests.
func (r *fakeRows) TypeMap() *pgtype.Map { return pgtype.NewMap() }

// Next advances to the next row.
func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}

	r.i++

	return true
}

// Scan assigns the current row values into dest via a type switch.
func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	row := r.rows[r.i-1]

	for j, d := range dest {
		if j >= len(row) {
			break
		}

		switch p := d.(type) {
		case *string:
			switch v := row[j].(type) {
			case string:
				*p = v
			case []byte:
				*p = string(v)
			}
		case *[]byte:
			switch v := row[j].(type) {
			case []byte:
				*p = v
			case string:
				*p = []byte(v)
			case nil:
				*p = nil
			}
		case *float64:
			switch v := row[j].(type) {
			case float64:
				*p = v
			case float32:
				*p = float64(v)
			case int:
				*p = float64(v)
			}
		}
	}

	return nil
}

// Values returns the current row values.
func (r *fakeRows) Values() ([]any, error) { return r.rows[r.i-1], nil }

// RawValues returns nil; fakes carry no raw bytes.
func (r *fakeRows) RawValues() [][]byte { return nil }

// Conn returns nil; fake rows are not bound to a connection.
func (r *fakeRows) Conn() *pgx.Conn { return nil }

// fakePool is an in-memory dbpool for unit tests without a live database.
type fakePool struct {
	execErrs  []error
	execCalls int
	execSQL   []string
	// execTag is returned by every Exec call whose index has no injected
	// error; the zero value's RowsAffected() is 0, matching pgconn's own
	// zero-value CommandTag{}.
	execTag   pgconn.CommandTag
	queryRows pgx.Rows
	queryErr  error
	querySQL  []string
	queryArgs [][]any
	closed    bool
}

// Exec records the SQL and returns the next injected error, or execTag.
func (p *fakePool) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	p.execSQL = append(p.execSQL, sql)

	var err error
	if p.execCalls < len(p.execErrs) {
		err = p.execErrs[p.execCalls]
	}

	p.execCalls++

	return p.execTag, err
}

// Query records the SQL and returns the stubbed rows or error.
func (p *fakePool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	p.querySQL = append(p.querySQL, sql)
	p.queryArgs = append(p.queryArgs, args)

	if p.queryErr != nil {
		return nil, p.queryErr
	}

	if p.queryRows != nil {
		return p.queryRows, nil
	}

	return &fakeRows{}, nil
}

// Close marks the pool closed.
func (p *fakePool) Close() { p.closed = true }

func TestTableDDL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dim  int
		want string
	}{
		{name: "default", dim: 0, want: "vector(1536)"},
		{name: "custom", dim: 3, want: "vector(3)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tableDDL(tt.dim)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("tableDDL(%d) = %q, want substring %q", tt.dim, got, tt.want)
			}

			if !strings.Contains(got, "CREATE TABLE IF NOT EXISTS vectors") {
				t.Fatalf("tableDDL(%d) = %q, missing vectors table", tt.dim, got)
			}
		})
	}
}

func TestParseVectorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     string
		want    int
		wantErr bool
	}{
		{name: "ok", typ: "vector(3)", want: 3},
		{name: "missing paren", typ: "vector", wantErr: true},
		{name: "bad int", typ: "vector(abc)", wantErr: true},
		{name: "zero", typ: "vector(0)", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVectorType(tt.typ)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseVectorType(%q) = %d, want error", tt.typ, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseVectorType(%q) error: %v", tt.typ, err)
			}

			if got != tt.want {
				t.Fatalf("parseVectorType(%q) = %d, want %d", tt.typ, got, tt.want)
			}
		})
	}
}

func TestCheckDimension(t *testing.T) {
	t.Parallel()

	if err := checkDimension(3, 3); err != nil {
		t.Fatalf("checkDimension equal error: %v", err)
	}

	err := checkDimension(2, 3)
	if err == nil {
		t.Fatal("checkDimension mismatch = nil, want error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "2") || !strings.Contains(msg, "3") {
		t.Fatalf("checkDimension message %q missing dimensions", msg)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("missing dsn", func(t *testing.T) {
		t.Parallel()

		_, err := New(vectorstore.Options{})
		if !errors.Is(err, ErrMissingDSN) {
			t.Fatalf("New() error = %v, want ErrMissingDSN", err)
		}
	})

	t.Run("invalid options", func(t *testing.T) {
		t.Parallel()

		_, err := New(vectorstore.Options{DSN: "postgres://localhost/db", Dimension: -1})
		if !errors.Is(err, vectorstore.ErrInvalidOptions) {
			t.Fatalf("New() error = %v, want ErrInvalidOptions", err)
		}
	})

	t.Run("default dimension no database", func(t *testing.T) {
		t.Parallel()

		_, err := New(vectorstore.Options{DSN: "postgres://127.0.0.1:1/zever_test"})
		if err == nil {
			t.Fatal("New() with unreachable DSN = nil, want error")
		}
	})
}

func TestNewFailure(t *testing.T) {
	t.Parallel()

	if _, err := New(vectorstore.Options{DSN: "://bad"}); err == nil {
		t.Fatal("New(bad DSN) = nil, want connect error")
	}

	if _, err := New(vectorstore.Options{DSN: "postgres://127.0.0.1:1/zever_test", Dimension: 3}); err == nil {
		t.Fatal("New(unreachable DSN) = nil, want error")
	}
}

func TestSetupStore(t *testing.T) {
	t.Parallel()

	t.Run("extension error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execErrs: []error{errors.New("boom")}}
		if _, err := setupStore(t.Context(), pool, 3); err == nil {
			t.Fatal("setupStore extension error = nil, want error")
		}

		if !pool.closed {
			t.Fatal("setupStore extension error left pool open")
		}
	})

	t.Run("inspect error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryErr: errors.New("boom")}
		_, err := setupStore(t.Context(), pool, 3)
		if err == nil || !strings.Contains(err.Error(), "inspect") {
			t.Fatalf("setupStore inspect error = %v, want inspect error", err)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"vector(2)"}}}}
		_, err := setupStore(t.Context(), pool, 3)
		if err == nil || !strings.Contains(err.Error(), "2") {
			t.Fatalf("setupStore mismatch error = %v, want dimension error", err)
		}

		if !pool.closed {
			t.Fatal("setupStore mismatch left pool open")
		}
	})

	t.Run("create table error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execErrs: []error{nil, errors.New("boom")}}
		_, err := setupStore(t.Context(), pool, 3)
		if err == nil || !strings.Contains(err.Error(), "create table") {
			t.Fatalf("setupStore create table error = %v, want create table error", err)
		}
	})

	t.Run("create index error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execErrs: []error{nil, nil, errors.New("boom")}}
		_, err := setupStore(t.Context(), pool, 3)
		if err == nil || !strings.Contains(err.Error(), "create index") {
			t.Fatalf("setupStore create index error = %v, want create index error", err)
		}
	})

	t.Run("ok new table", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s, err := setupStore(t.Context(), pool, 3)
		if err != nil {
			t.Fatalf("setupStore error: %v", err)
		}

		if s.dim != 3 {
			t.Fatalf("setupStore dim = %d, want 3", s.dim)
		}

		if pool.closed {
			t.Fatal("setupStore ok closed pool")
		}

		joined := strings.Join(pool.execSQL, "\n")
		if !strings.Contains(joined, "CREATE EXTENSION") || !strings.Contains(joined, "ivfflat") {
			t.Fatalf("setupStore SQL = %q, want extension and ivfflat", joined)
		}
	})

	t.Run("ok existing table", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"vector(3)"}}}}
		s, err := setupStore(t.Context(), pool, 3)
		if err != nil {
			t.Fatalf("setupStore error: %v", err)
		}

		if s.dim != 3 {
			t.Fatalf("setupStore dim = %d, want 3", s.dim)
		}
	})
}

func TestEmbeddingColumnDim(t *testing.T) {
	t.Parallel()

	t.Run("no table", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{}}
		dim, exists, err := embeddingColumnDim(t.Context(), pool)
		if err != nil || exists || dim != 0 {
			t.Fatalf("embeddingColumnDim() = (%d,%v,%v), want (0,false,nil)", dim, exists, err)
		}
	})

	t.Run("no table rows error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{err: errors.New("boom")}}
		if _, _, err := embeddingColumnDim(t.Context(), pool); err == nil {
			t.Fatal("embeddingColumnDim() = nil, want rows error")
		}
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"vector(3)"}}}}
		dim, exists, err := embeddingColumnDim(t.Context(), pool)
		if err != nil || !exists || dim != 3 {
			t.Fatalf("embeddingColumnDim() = (%d,%v,%v), want (3,true,nil)", dim, exists, err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryErr: errors.New("boom")}
		if _, _, err := embeddingColumnDim(t.Context(), pool); err == nil {
			t.Fatal("embeddingColumnDim() = nil, want query error")
		}
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"vector(3)"}}, scanErr: errors.New("boom")}}
		if _, _, err := embeddingColumnDim(t.Context(), pool); err == nil {
			t.Fatal("embeddingColumnDim() = nil, want scan error")
		}
	})

	t.Run("trailing rows error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"vector(3)"}}, err: errors.New("boom")}}
		if _, _, err := embeddingColumnDim(t.Context(), pool); err == nil {
			t.Fatal("embeddingColumnDim() = nil, want trailing rows error")
		}
	})

	t.Run("parse error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"text"}}}}
		if _, _, err := embeddingColumnDim(t.Context(), pool); err == nil {
			t.Fatal("embeddingColumnDim() = nil, want parse error")
		}
	})
}

func TestUpsert(t *testing.T) {
	t.Parallel()

	vec := vectorstore.Vector{ID: "a", Embedding: []float32{1, 0, 0}, Metadata: map[string]any{"k": "v"}}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 3}
		if err := s.Upsert(t.Context(), vec); err != nil {
			t.Fatalf("Upsert error: %v", err)
		}

		sql := pool.execSQL[0]
		if !strings.Contains(sql, "ON CONFLICT") || !strings.Contains(sql, "$2::vector") {
			t.Fatalf("Upsert SQL = %q, want ON CONFLICT and $2::vector", sql)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 3}
		err := s.Upsert(t.Context(), vectorstore.Vector{ID: "a", Embedding: []float32{1, 0}})
		var mismatch *vectorstore.DimensionMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("Upsert error = %v, want DimensionMismatchError", err)
		}

		if mismatch.Got != 2 || mismatch.Want != 3 {
			t.Fatalf("Upsert mismatch = %+v, want got 2 want 3", mismatch)
		}
	})

	t.Run("metadata marshal error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 3}
		bad := vectorstore.Vector{ID: "a", Embedding: []float32{1, 0, 0}, Metadata: map[string]any{"f": func() {}}}
		if err := s.Upsert(t.Context(), bad); err == nil {
			t.Fatal("Upsert bad metadata = nil, want marshal error")
		}
	})

	t.Run("embedding marshal error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 1}
		bad := vectorstore.Vector{ID: "a", Embedding: []float32{float32(math.NaN())}}
		if err := s.Upsert(t.Context(), bad); err == nil {
			t.Fatal("Upsert NaN embedding = nil, want marshal error")
		}
	})

	t.Run("exec error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execErrs: []error{errors.New("boom")}}
		s := &Store{db: pool, dim: 3}
		if err := s.Upsert(t.Context(), vec); err == nil {
			t.Fatal("Upsert exec error = nil, want error")
		}
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execTag: pgconn.NewCommandTag("DELETE 1")}
		s := &Store{db: pool, dim: 3}
		if err := s.Delete(t.Context(), "a"); err != nil {
			t.Fatalf("Delete error: %v", err)
		}

		if !strings.Contains(pool.execSQL[0], "$1") {
			t.Fatalf("Delete SQL = %q, want $1", pool.execSQL[0])
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execTag: pgconn.NewCommandTag("DELETE 0")}
		s := &Store{db: pool, dim: 3}
		err := s.Delete(t.Context(), "missing")
		var notFound *vectorstore.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Delete error = %v, want NotFoundError", err)
		}

		if notFound.ID != "missing" {
			t.Fatalf("Delete ID = %q, want missing", notFound.ID)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{execErrs: []error{errors.New("boom")}}
		s := &Store{db: pool, dim: 3}
		if err := s.Delete(t.Context(), "a"); err == nil {
			t.Fatal("Delete exec error = nil, want error")
		}
	})
}

func TestQuery(t *testing.T) {
	t.Parallel()

	rowsFor := func() *fakeRows {
		return &fakeRows{rows: [][]any{
			{"a", float64(0.9), []byte(`{"k":"v"}`)},
			{"b", float64(0.1), nil},
		}}
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: rowsFor()}
		s := &Store{db: pool, dim: 3}
		got, err := s.Query(t.Context(), []float32{1, 0, 0}, 2)
		if err != nil {
			t.Fatalf("Query error: %v", err)
		}

		if len(got) != 2 || got[0].ID != "a" || got[0].Score != float32(0.9) {
			t.Fatalf("Query = %+v, want 2 matches starting with a/0.9", got)
		}

		if got[0].Metadata["k"] != "v" {
			t.Fatalf("Query metadata = %+v, want k=v", got[0].Metadata)
		}

		if got[1].Metadata != nil {
			t.Fatalf("Query nil metadata = %+v, want nil", got[1].Metadata)
		}

		sql := pool.querySQL[0]
		if !strings.Contains(sql, "<=>") || !strings.Contains(sql, "LIMIT") || !strings.Contains(sql, "$1::vector") {
			t.Fatalf("Query SQL = %q, want <=>, LIMIT and $1::vector", sql)
		}
	})

	t.Run("default topk", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: rowsFor()}
		s := &Store{db: pool, dim: 3}
		if _, err := s.Query(t.Context(), []float32{1, 0, 0}, 0); err != nil {
			t.Fatalf("Query error: %v", err)
		}

		args := pool.queryArgs[0]
		if args[2] != vectorstore.DefaultTopK {
			t.Fatalf("Query topK arg = %v, want %d", args[2], vectorstore.DefaultTopK)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{}}
		s := &Store{db: pool, dim: 3}
		got, err := s.Query(t.Context(), []float32{1, 0, 0}, 2)
		if err != nil || len(got) != 0 {
			t.Fatalf("Query empty = (%v,%v), want (empty,nil)", got, err)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 3}
		_, err := s.Query(t.Context(), []float32{1, 0}, 2)
		var mismatch *vectorstore.DimensionMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("Query error = %v, want DimensionMismatchError", err)
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{}
		s := &Store{db: pool, dim: 1}
		_, err := s.Query(t.Context(), []float32{float32(math.NaN())}, 2)
		if err == nil {
			t.Fatal("Query NaN embedding = nil, want marshal error")
		}
	})

	t.Run("query error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryErr: errors.New("boom")}
		s := &Store{db: pool, dim: 3}
		if _, err := s.Query(t.Context(), []float32{1, 0, 0}, 2); err == nil {
			t.Fatal("Query query error = nil, want error")
		}
	})

	t.Run("scan error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"a", float64(1), nil}}, scanErr: errors.New("boom")}}
		s := &Store{db: pool, dim: 3}
		if _, err := s.Query(t.Context(), []float32{1, 0, 0}, 2); err == nil {
			t.Fatal("Query scan error = nil, want error")
		}
	})

	t.Run("metadata decode error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{rows: [][]any{{"a", float64(1), []byte(`{bad`)}}}}
		s := &Store{db: pool, dim: 3}
		if _, err := s.Query(t.Context(), []float32{1, 0, 0}, 2); err == nil {
			t.Fatal("Query metadata decode error = nil, want error")
		}
	})

	t.Run("rows error", func(t *testing.T) {
		t.Parallel()

		pool := &fakePool{queryRows: &fakeRows{err: errors.New("boom")}}
		s := &Store{db: pool, dim: 3}
		if _, err := s.Query(t.Context(), []float32{1, 0, 0}, 2); err == nil {
			t.Fatal("Query rows error = nil, want error")
		}
	})
}

func TestRowsCloseErr(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		if err := rowsCloseErr(&fakeRows{}); err != nil {
			t.Fatalf("rowsCloseErr() = %v, want nil", err)
		}
	})

	t.Run("rows error", func(t *testing.T) {
		t.Parallel()

		if err := rowsCloseErr(&fakeRows{err: errors.New("boom")}); err == nil {
			t.Fatal("rowsCloseErr() = nil, want rows error")
		}
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	pool := &fakePool{}
	s := &Store{db: pool, dim: 3}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	if !pool.closed {
		t.Fatal("Close() left pool open")
	}
}

func TestUpsertBatch_Empty(t *testing.T) {
	t.Parallel()

	pool := &fakePool{}
	s := &Store{db: pool, dim: 3}

	if err := s.UpsertBatch(t.Context(), nil); err != nil {
		t.Fatalf("UpsertBatch(nil) = %v, want nil", err)
	}

	if len(pool.execSQL) != 0 {
		t.Fatalf("UpsertBatch(nil) issued %d exec calls, want 0", len(pool.execSQL))
	}
}

func TestUpsertBatch_SingleStatement(t *testing.T) {
	t.Parallel()

	pool := &fakePool{}
	s := &Store{db: pool, dim: 3}

	err := s.UpsertBatch(t.Context(), []vectorstore.Vector{
		{ID: "a", Embedding: []float32{1, 0, 0}},
		{ID: "b", Embedding: []float32{0, 1, 0}},
	})
	if err != nil {
		t.Fatalf("UpsertBatch() = %v, want nil", err)
	}

	if len(pool.execSQL) != 1 {
		t.Fatalf("UpsertBatch() issued %d exec calls, want 1", len(pool.execSQL))
	}

	if !strings.Contains(pool.execSQL[0], "$4") {
		t.Fatalf("UpsertBatch() sql = %q, want multi-row placeholders", pool.execSQL[0])
	}
}

func TestUpsertBatch_DimensionMismatch(t *testing.T) {
	t.Parallel()

	pool := &fakePool{}
	s := &Store{db: pool, dim: 3}

	err := s.UpsertBatch(t.Context(), []vectorstore.Vector{
		{ID: "a", Embedding: []float32{1, 0}},
	})

	var mismatch *vectorstore.DimensionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("UpsertBatch() = %v, want DimensionMismatchError", err)
	}

	if len(pool.execSQL) != 0 {
		t.Fatalf("UpsertBatch() issued exec on bad dimension, want none")
	}
}

// TestUpsertBatch_Chunked proves a batch larger than maxUpsertBatchRows is
// split into multiple sequential INSERT statements instead of one unbounded
// statement that could overflow Postgres's bind-parameter limit.
func TestUpsertBatch_Chunked(t *testing.T) {
	t.Parallel()

	pool := &fakePool{}
	s := &Store{db: pool, dim: 3}

	const total = maxUpsertBatchRows + 500

	vecs := make([]vectorstore.Vector, total)
	for i := range vecs {
		vecs[i] = vectorstore.Vector{ID: fmt.Sprintf("id-%d", i), Embedding: []float32{1, 0, 0}}
	}

	if err := s.UpsertBatch(t.Context(), vecs); err != nil {
		t.Fatalf("UpsertBatch() = %v, want nil", err)
	}

	wantChunks := 2
	if len(pool.execSQL) != wantChunks {
		t.Fatalf("UpsertBatch() issued %d exec calls, want %d", len(pool.execSQL), wantChunks)
	}

	// First chunk carries maxUpsertBatchRows rows (upsertRowCols placeholders
	// each), the second carries the 500-row remainder.
	firstArgsWant := maxUpsertBatchRows * upsertRowCols
	secondArgsWant := 500 * upsertRowCols

	if !strings.Contains(pool.execSQL[0], fmt.Sprintf("$%d", firstArgsWant)) {
		t.Fatalf("first chunk SQL missing final placeholder $%d", firstArgsWant)
	}

	if !strings.Contains(pool.execSQL[1], fmt.Sprintf("$%d", secondArgsWant)) {
		t.Fatalf("second chunk SQL missing final placeholder $%d", secondArgsWant)
	}
}

func TestLiveRoundTrip(t *testing.T) {
	dsn := os.Getenv("PGVECTOR_TEST_DSN")
	if dsn == "" {
		t.Skip("PGVECTOR_TEST_DSN unset")
	}

	ctx := t.Context()

	vs, err := New(vectorstore.Options{DSN: dsn, Dimension: 3})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := vs.Close(); closeErr != nil {
			t.Errorf("Close() error: %v", closeErr)
		}
	})

	if upsertErr := vs.Upsert(ctx, vectorstore.Vector{ID: "a", Embedding: []float32{1, 0, 0}}); upsertErr != nil {
		t.Fatalf("Upsert(a) error: %v", upsertErr)
	}

	if upsertErr := vs.Upsert(ctx, vectorstore.Vector{ID: "b", Embedding: []float32{0, 1, 0}}); upsertErr != nil {
		t.Fatalf("Upsert(b) error: %v", upsertErr)
	}

	got, err := vs.Query(ctx, []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}

	if len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("Query() = %+v, want a first", got)
	}

	if got[0].Score < 0.99 {
		t.Fatalf("Query() identical score = %v, want ~1", got[0].Score)
	}

	if got[1].Score > 0.01 {
		t.Fatalf("Query() orthogonal score = %v, want ~0", got[1].Score)
	}

	if err := vs.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete(a) error: %v", err)
	}

	var notFound *vectorstore.NotFoundError
	if err := vs.Delete(ctx, "a"); !errors.As(err, &notFound) {
		t.Fatalf("Delete(a) again = %v, want NotFoundError", err)
	}

	var mismatch *vectorstore.DimensionMismatchError
	if err := vs.Upsert(ctx, vectorstore.Vector{ID: "c", Embedding: []float32{1, 0}}); !errors.As(err, &mismatch) {
		t.Fatalf("Upsert wrong dim = %v, want DimensionMismatchError", err)
	}
}

// TestLiveUpsertBatch_MatchesLoopedUpsert proves UpsertBatch produces the same
// end-state as calling Upsert N times in a loop, against a live pgvector
// instance. Skipped when PGVECTOR_TEST_DSN is unset, matching TestLiveRoundTrip.
func TestLiveUpsertBatch_MatchesLoopedUpsert(t *testing.T) {
	dsn := os.Getenv("PGVECTOR_TEST_DSN")
	if dsn == "" {
		t.Skip("PGVECTOR_TEST_DSN unset")
	}

	ctx := t.Context()

	loopStore, err := New(vectorstore.Options{DSN: dsn, Dimension: 3})
	if err != nil {
		t.Fatalf("Open() loop store error: %v", err)
	}

	t.Cleanup(func() { _ = loopStore.Close() })

	vecs := []vectorstore.Vector{
		{ID: "batch-a", Embedding: []float32{1, 0, 0}, Metadata: map[string]any{"n": "one"}},
		{ID: "batch-b", Embedding: []float32{0, 1, 0}, Metadata: map[string]any{"n": "two"}},
		{ID: "batch-c", Embedding: []float32{0, 0, 1}, Metadata: map[string]any{"n": "three"}},
	}

	for _, v := range vecs {
		if upsertErr := loopStore.Upsert(ctx, v); upsertErr != nil {
			t.Fatalf("loop Upsert(%s) error: %v", v.ID, upsertErr)
		}
	}

	t.Cleanup(func() {
		for _, v := range vecs {
			_ = loopStore.Delete(ctx, v.ID)
		}
	})

	batchStore, err := New(vectorstore.Options{DSN: dsn, Dimension: 3})
	if err != nil {
		t.Fatalf("New() batch store error: %v", err)
	}

	t.Cleanup(func() { _ = batchStore.Close() })

	if batchErr := batchStore.UpsertBatch(ctx, vecs); batchErr != nil {
		t.Fatalf("UpsertBatch() error: %v", batchErr)
	}

	loopGot, err := loopStore.Query(ctx, []float32{1, 0, 0}, 3)
	if err != nil {
		t.Fatalf("loop Query() error: %v", err)
	}

	batchGot, err := batchStore.Query(ctx, []float32{1, 0, 0}, 3)
	if err != nil {
		t.Fatalf("batch Query() error: %v", err)
	}

	if len(loopGot) != len(batchGot) {
		t.Fatalf("result count mismatch: loop=%d batch=%d", len(loopGot), len(batchGot))
	}

	for i := range loopGot {
		if loopGot[i].ID != batchGot[i].ID {
			t.Fatalf("id mismatch at %d: loop=%s batch=%s", i, loopGot[i].ID, batchGot[i].ID)
		}
	}
}
