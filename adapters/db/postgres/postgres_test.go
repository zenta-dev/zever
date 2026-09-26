package postgres

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenta-dev/zever/core/db"
)

const deadDSN = "postgres://127.0.0.1:1/db?sslmode=disable"

func concretePool(t *testing.T, d db.DB) *pgxpool.Pool {
	t.Helper()

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("want *adapter, got %T", d)
	}

	p, ok := a.pool.(*pgxpool.Pool)
	if !ok {
		t.Fatalf("want *pgxpool.Pool, got %T", a.pool)
	}

	return p
}

func openDead(t *testing.T) db.DB {
	t.Helper()

	d, err := New(db.Options{DSN: deadDSN})
	if err != nil {
		t.Fatalf("New(dead): %v", err)
	}

	t.Cleanup(func() {
		_ = d.Close(t.Context())
	})

	return d
}

func shortCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithTimeout(t.Context(), 2*time.Second)
}

func TestNew_missingDSN(t *testing.T) {
	t.Parallel()

	if _, err := New(db.Options{}); err == nil {
		t.Fatal("want error for missing dsn, got nil")
	}
}

func TestNew_whitespaceDSN(t *testing.T) {
	t.Parallel()

	if _, err := New(db.Options{DSN: "   "}); err == nil {
		t.Fatal("want error for whitespace dsn, got nil")
	}
}

func TestNew_badSyntax(t *testing.T) {
	t.Parallel()

	if _, err := New(db.Options{DSN: "not a dsn ;;;"}); err == nil {
		t.Fatal("want error for bad syntax, got nil")
	}
}

func TestNew_poolKnobs(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{
		DSN:             "postgres://127.0.0.1:1/db?sslmode=disable",
		MaxConns:        7,
		MinConns:        2,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("want *adapter, got %T", d)
	}

	realPool, ok := a.pool.(*pgxpool.Pool)
	if !ok {
		t.Fatalf("want *pgxpool.Pool, got %T", a.pool)
	}

	cfg := realPool.Config()
	if cfg.MaxConns != 7 {
		t.Errorf("MaxConns = %d, want 7", cfg.MaxConns)
	}

	if cfg.MinConns != 2 {
		t.Errorf("MinConns = %d, want 2", cfg.MinConns)
	}

	if cfg.MaxConnLifetime != time.Minute {
		t.Errorf("MaxConnLifetime = %v, want 1m", cfg.MaxConnLifetime)
	}

	if cfg.MaxConnIdleTime != 30*time.Second {
		t.Errorf("MaxConnIdleTime = %v, want 30s", cfg.MaxConnIdleTime)
	}

	if got := realPool.Stat().MaxConns(); got != 7 {
		t.Errorf("Stat().MaxConns() = %d, want 7", got)
	}
}

func TestNew_poolKnobsZeroLeavesDefaults(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=disable"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("want *adapter, got %T", d)
	}

	realPool, ok := a.pool.(*pgxpool.Pool)
	if !ok {
		t.Fatalf("want *pgxpool.Pool, got %T", a.pool)
	}

	if realPool.Config().MaxConns == 0 {
		t.Error("want pgxpool default MaxConns, got 0")
	}
}

func TestNew_newWithConfigError(t *testing.T) {
	t.Parallel()

	// int32 overflow wraps negative, puddle rejects MaxSize < 1.
	if _, err := New(db.Options{DSN: deadDSN, MaxConns: 1 << 31}); err == nil {
		t.Fatal("want error for overflowing MaxConns, got nil")
	}
}

func TestTLS_defaultFloor(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	cfg := concretePool(t, d).Config()
	if cfg.ConnConfig.TLSConfig == nil {
		t.Fatal("want TLSConfig non-nil without sslmode")
	}

	if cfg.ConnConfig.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS1.2", cfg.ConnConfig.TLSConfig.MinVersion)
	}
}

func TestTLS_disableNil(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=disable"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	if cfg := concretePool(t, d).Config(); cfg.ConnConfig.TLSConfig != nil {
		t.Fatalf("want nil TLSConfig for sslmode=disable, got %+v", cfg.ConnConfig.TLSConfig)
	}
}

func TestTLS_requireFloor(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=require"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	cfg := concretePool(t, d).Config()
	if cfg.ConnConfig.TLSConfig == nil {
		t.Fatal("want TLSConfig for sslmode=require")
	}

	if cfg.ConnConfig.TLSConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("MinVersion %x < TLS1.2", cfg.ConnConfig.TLSConfig.MinVersion)
	}
}

func TestTLS_envDisableNil(t *testing.T) {
	t.Setenv("PGSSLMODE", "disable")

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	if cfg := concretePool(t, d).Config(); cfg.ConnConfig.TLSConfig != nil {
		t.Fatalf("want nil TLSConfig for PGSSLMODE=disable, got %+v", cfg.ConnConfig.TLSConfig)
	}
}

func TestTLS_envRequireFloor(t *testing.T) {
	t.Setenv("PGSSLMODE", "require")

	d, err := New(db.Options{DSN: "postgres://127.0.0.1:1/db"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	cfg := concretePool(t, d).Config()
	if cfg.ConnConfig.TLSConfig == nil {
		t.Fatal("want TLSConfig for PGSSLMODE=require")
	}

	if cfg.ConnConfig.TLSConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("MinVersion %x < TLS1.2", cfg.ConnConfig.TLSConfig.MinVersion)
	}
}

func assertNetOpError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("want error, got nil")
	}

	if errors.Unwrap(err) == nil {
		t.Fatal("want wrapped error chain, got nil Unwrap")
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("want *net.OpError in chain, got %T: %v", err, err)
	}
}

func TestFailFast_query(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	ctx, cancel := shortCtx(t)
	defer cancel()

	_, err := d.Query(ctx, "SELECT 1 WHERE 1 = ?", 1)
	assertNetOpError(t, err)
}

func TestFailFast_exec(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	ctx, cancel := shortCtx(t)
	defer cancel()

	_, err := d.Exec(ctx, "SELECT 1 WHERE 1 = ?", 1)
	assertNetOpError(t, err)
}

func TestFailFast_ping(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	ctx, cancel := shortCtx(t)
	defer cancel()

	assertNetOpError(t, d.Ping(ctx))
}

func TestFailFast_beginTx(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	ctx, cancel := shortCtx(t)
	defer cancel()

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("want db.Transactor, got %T", d)
	}

	_, err := tr.BeginTx(ctx, nil)
	assertNetOpError(t, err)
}

func TestBeginTx_isolationMapping(t *testing.T) {
	t.Parallel()

	d := openDead(t)

	cases := []struct {
		name string
		opts *db.TxOptions
	}{
		{"nil defaults", nil},
		{"read committed", &db.TxOptions{Isolation: db.ReadCommitted}},
		{"repeatable read", &db.TxOptions{Isolation: db.RepeatableRead}},
		{"serializable", &db.TxOptions{Isolation: db.Serializable}},
		{"unknown defaults", &db.TxOptions{Isolation: db.IsolationLevel(99)}},
		{"readonly deferrable", &db.TxOptions{Isolation: db.Serializable, ReadOnly: true, Deferrable: true}},
		{"readonly", &db.TxOptions{ReadOnly: true}},
	}

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("want db.Transactor, got %T", d)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := shortCtx(t)
			defer cancel()

			_, err := tr.BeginTx(ctx, tc.opts)
			assertNetOpError(t, err)
		})
	}
}

func TestDialect(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	if got := d.Dialect(); got != "postgres" {
		t.Fatalf("Dialect = %q, want postgres", got)
	}

	tx := &pgTx{tx: &fakeTx{}}
	if got := tx.Dialect(); got != "postgres" {
		t.Fatalf("tx Dialect = %q, want postgres", got)
	}
}

func TestClose_canceledCtx(t *testing.T) {
	t.Parallel()

	d, err := New(db.Options{DSN: deadDSN})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := d.Close(ctx); err == nil {
		t.Fatal("want error for canceled ctx, got nil")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled in chain, got %v", err)
	}
}

func TestClose_ok(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	if err := d.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWithTx_beginErrorIsTxError(t *testing.T) {
	t.Parallel()

	d := openDead(t)
	ctx, cancel := shortCtx(t)
	defer cancel()

	err := db.WithTx(ctx, d, nil, func(context.Context, db.Tx) error {
		t.Error("fn should not run")

		return nil
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var txErr *db.TxError
	if !errors.As(err, &txErr) {
		t.Fatalf("want *db.TxError, got %T", err)
	}

	if txErr.Op != "begin" {
		t.Fatalf("Op = %q, want begin", txErr.Op)
	}
}

// fakeRows implements pgx.Rows without a server.
type fakeRows struct {
	nextVals []bool
	nextIdx  int
	scanErr  error
	closeErr error
	cols     []pgconn.FieldDescription
}

func (f *fakeRows) Close() {}

func (f *fakeRows) Err() error { return f.closeErr }

func (f *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.NewCommandTag("SELECT 1") }

func (f *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return f.cols }

func (f *fakeRows) Next() bool {
	if f.nextIdx >= len(f.nextVals) {
		return false
	}

	v := f.nextVals[f.nextIdx]
	f.nextIdx++

	return v
}

func (f *fakeRows) Scan(...any) error { return f.scanErr }

func (f *fakeRows) Values() ([]any, error) { return nil, nil }

func (f *fakeRows) RawValues() [][]byte { return nil }

// TypeMap returns a fresh type map; the adapter never consults it in tests.
func (f *fakeRows) TypeMap() *pgtype.Map { return pgtype.NewMap() }

func (f *fakeRows) Conn() *pgx.Conn { return nil }

func TestRowsAdapter_nextScanColumns(t *testing.T) {
	t.Parallel()

	r := &rowsAdapter{rows: &fakeRows{
		nextVals: []bool{true, false},
		cols:     []pgconn.FieldDescription{{Name: "id"}, {Name: "name"}},
	}}

	if !r.Next() {
		t.Fatal("want Next true")
	}

	if err := r.Scan(new(int)); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	cols, err := r.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}

	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("cols = %v, want [id name]", cols)
	}

	if r.Next() {
		t.Fatal("want Next false")
	}

	if err := r.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRowsAdapter_closeError(t *testing.T) {
	t.Parallel()

	r := &rowsAdapter{rows: &fakeRows{closeErr: errors.New("boom")}}
	if err := r.Close(); err == nil {
		t.Fatal("want close error, got nil")
	}
}

func TestRowsAdapter_scanError(t *testing.T) {
	t.Parallel()

	r := &rowsAdapter{rows: &fakeRows{scanErr: errors.New("scan boom")}}
	if err := r.Scan(new(int)); err == nil {
		t.Fatal("want scan error, got nil")
	}
}

// fakeTx implements pgx.Tx without a server.
type fakeTx struct {
	commitErr   error
	rollbackErr error
	queryRows   pgx.Rows
	queryErr    error
	execTag     pgconn.CommandTag
	execErr     error
	execCalls   []string
}

func (f *fakeTx) Begin(_ context.Context) (pgx.Tx, error) { return f, nil }

func (f *fakeTx) Commit(_ context.Context) error { return f.commitErr }

func (f *fakeTx) Rollback(_ context.Context) error { return f.rollbackErr }

func (f *fakeTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (f *fakeTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }

func (f *fakeTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (f *fakeTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return &pgconn.StatementDescription{}, nil
}

func (f *fakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execCalls = append(f.execCalls, sql)

	return f.execTag, f.execErr
}

func (f *fakeTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}

	return f.queryRows, nil
}

func (f *fakeTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }

func (f *fakeTx) Conn() *pgx.Conn { return nil }

func TestPgTx_commitOnce(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{}}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := tx.Commit(t.Context()); err == nil {
		t.Fatal("want error on double commit, got nil")
	}
}

func TestPgTx_commitError(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{commitErr: errors.New("commit down")}}
	if err := tx.Commit(t.Context()); err == nil {
		t.Fatal("want commit error, got nil")
	}

	// Failed commit leaves tx open, second attempt surfaces same error.
	if err := tx.Commit(t.Context()); err == nil {
		t.Fatal("want second commit error, got nil")
	}
}

func TestPgTx_rollbackIdempotent(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{}}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("second Rollback: %v", err)
	}
}

func TestPgTx_rollbackError(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{rollbackErr: errors.New("rollback down")}}
	if err := tx.Rollback(t.Context()); err == nil {
		t.Fatal("want rollback error, got nil")
	}
}

func TestPgTx_closeRollsBackOnce(t *testing.T) {
	t.Parallel()

	fake := &fakeTx{}
	tx := &pgTx{tx: fake}

	if err := tx.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := tx.Close(t.Context()); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if err := tx.Commit(t.Context()); err == nil {
		t.Fatal("want commit-after-close error, got nil")
	}
}

func TestPgTx_closeAfterCommit(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{}}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := tx.Close(t.Context()); err != nil {
		t.Fatalf("Close after commit: %v", err)
	}
}

func TestPgTx_closeRollbackError(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{rollbackErr: errors.New("rb down")}}
	if err := tx.Close(t.Context()); err == nil {
		t.Fatal("want close error, got nil")
	}
}

func TestPgTx_queryExec(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{
		queryRows: &fakeRows{nextVals: []bool{false}},
		execTag:   pgconn.NewCommandTag("UPDATE 2"),
	}}

	rows, err := tx.Query(t.Context(), "SELECT 1 WHERE 1 = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	_ = rows.Close()

	n, err := tx.Exec(t.Context(), "UPDATE t SET a = ? WHERE b = ?", 1, 2)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if n != 2 {
		t.Fatalf("rows affected = %d, want 2", n)
	}
}

func TestPgTx_queryError(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{queryErr: errors.New("query down")}}
	if _, err := tx.Query(t.Context(), "SELECT 1"); err == nil {
		t.Fatal("want query error, got nil")
	}
}

func TestPgTx_execError(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{execErr: errors.New("exec down")}}
	if _, err := tx.Exec(t.Context(), "SELECT 1"); err == nil {
		t.Fatal("want exec error, got nil")
	}
}

func TestPgTx_pingDone(t *testing.T) {
	t.Parallel()

	tx := &pgTx{tx: &fakeTx{}}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := tx.Ping(t.Context()); err == nil {
		t.Fatal("want ping-after-close error, got nil")
	}
}

func TestPgTx_savepoint(t *testing.T) {
	t.Parallel()

	fake := &fakeTx{}
	tx := &pgTx{tx: fake}

	if err := tx.Savepoint(t.Context(), "sp1"); err != nil {
		t.Fatalf("Savepoint: %v", err)
	}

	if len(fake.execCalls) != 1 || fake.execCalls[0] != `SAVEPOINT "sp1"` {
		t.Fatalf("execCalls = %v, want SAVEPOINT quoted", fake.execCalls)
	}

	if err := tx.RollbackTo(t.Context(), "sp1"); err != nil {
		t.Fatalf("RollbackTo: %v", err)
	}

	if len(fake.execCalls) != 2 || fake.execCalls[1] != `ROLLBACK TO SAVEPOINT "sp1"` {
		t.Fatalf("execCalls = %v, want rollback quoted", fake.execCalls)
	}
}

func TestPgTx_savepointErrors(t *testing.T) {
	t.Parallel()

	bad := []string{"", "1abc", "a-b", "a b", "a.b"}
	for _, name := range bad {
		tx := &pgTx{tx: &fakeTx{}}
		if err := tx.Savepoint(t.Context(), name); err == nil {
			t.Fatalf("Savepoint(%q): want error, got nil", name)
		}

		if err := tx.RollbackTo(t.Context(), name); err == nil {
			t.Fatalf("RollbackTo(%q): want error, got nil", name)
		}
	}

	tx := &pgTx{tx: &fakeTx{execErr: errors.New("down")}}
	if err := tx.Savepoint(t.Context(), "ok"); err == nil {
		t.Fatal("want savepoint exec error, got nil")
	}

	tx2 := &pgTx{tx: &fakeTx{execErr: errors.New("down")}}
	if err := tx2.RollbackTo(t.Context(), "ok"); err == nil {
		t.Fatal("want rollback-to exec error, got nil")
	}
}

func TestQuoteIdent(t *testing.T) {
	t.Parallel()

	if got := quoteIdent(`a"b`); got != `"a""b"` {
		t.Fatalf("quoteIdent = %q, want %q", got, `"a""b"`)
	}
}

func TestValidateSavepointName(t *testing.T) {
	t.Parallel()

	if err := validateSavepointName("_ok123"); err != nil {
		t.Fatalf("valid name: %v", err)
	}

	if err := validateSavepointName("école"); err != nil {
		t.Fatalf("unicode name: %v", err)
	}
}

func TestIntegrationCRUD(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres integration tests")
	}

	ctx := t.Context()

	d, err := New(db.Options{DSN: dsn})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(ctx) }()

	err = d.Ping(ctx)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}

	_, err = d.Exec(ctx, `DROP TABLE IF EXISTS zever_pg_test`)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}

	_, err = d.Exec(ctx, `CREATE TABLE zever_pg_test (id BIGSERIAL PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Cleanup(func() {
		_, _ = d.Exec(ctx, `DROP TABLE zever_pg_test`)
	})

	n, err := d.Exec(ctx, `INSERT INTO zever_pg_test (name) VALUES (?), (?)`, "alice", "bob")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if n != 2 {
		t.Fatalf("insert rows = %d, want 2", n)
	}

	rows, err := d.Query(ctx, `SELECT name FROM zever_pg_test ORDER BY name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	var names []string

	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			_ = rows.Close()
			t.Fatalf("scan: %v", err)
		}

		names = append(names, name)
	}

	err = rows.Close()
	if err != nil {
		t.Fatalf("close rows: %v", err)
	}

	if len(names) != 2 || names[0] != "alice" || names[1] != "bob" {
		t.Fatalf("names = %v, want [alice bob]", names)
	}

	cols, err := func() ([]string, error) {
		r, qerr := d.Query(ctx, `SELECT id, name FROM zever_pg_test LIMIT 1`)
		if qerr != nil {
			return nil, qerr
		}
		defer func() { _ = r.Close() }()

		return r.Columns()
	}()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}

	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("cols = %v, want [id name]", cols)
	}

	transactor, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("want db.Transactor, got %T", d)
	}

	tr, err := transactor.BeginTx(ctx, &db.TxOptions{Isolation: db.Serializable})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	err = tr.Savepoint(ctx, "sp1")
	if err != nil {
		_ = tr.Rollback(ctx)
		t.Fatalf("savepoint: %v", err)
	}

	_, err = tr.Exec(ctx, `INSERT INTO zever_pg_test (name) VALUES (?)`, "carol")
	if err != nil {
		_ = tr.Rollback(ctx)
		t.Fatalf("tx insert: %v", err)
	}

	err = tr.RollbackTo(ctx, "sp1")
	if err != nil {
		_ = tr.Rollback(ctx)
		t.Fatalf("rollback to: %v", err)
	}

	err = tr.Commit(ctx)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	count := 0
	r2, err := d.Query(ctx, `SELECT name FROM zever_pg_test`)
	if err != nil {
		t.Fatalf("query after tx: %v", err)
	}

	for r2.Next() {
		count++
	}

	_ = r2.Close()

	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

type fakePool struct {
	rows    pgx.Rows
	rowsErr error
	tag     pgconn.CommandTag
	execErr error
	pingErr error
	tx      pgx.Tx
	txErr   error
	seen    []string
	closed  bool
}

func (f *fakePool) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	f.seen = append(f.seen, query)
	if f.rowsErr != nil {
		return nil, f.rowsErr
	}
	return f.rows, nil
}

func (f *fakePool) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	f.seen = append(f.seen, query)
	if f.execErr != nil {
		return f.tag, f.execErr
	}
	return f.tag, nil
}

func (f *fakePool) Ping(context.Context) error { return f.pingErr }

func (f *fakePool) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	if f.txErr != nil {
		return nil, f.txErr
	}
	return f.tx, nil
}

func (f *fakePool) Close() { f.closed = true }

type fakeConn struct{ err error }

func (f *fakeConn) Ping(context.Context) error { return f.err }

func TestAdapterQuery_success(t *testing.T) {
	t.Parallel()

	fp := &fakePool{rows: &fakeRows{
		nextVals: []bool{true, false},
		cols:     []pgconn.FieldDescription{{Name: "id"}},
	}}
	a := &adapter{pool: fp}

	rows, err := a.Query(t.Context(), "SELECT id FROM t WHERE id = ?", "42")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !rows.Next() {
		t.Fatal("want one row")
	}
	err = rows.Scan(new(int))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if rows.Next() {
		t.Fatal("want exactly one row")
	}
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 1 || cols[0] != "id" {
		t.Fatalf("cols = %v, want [id]", cols)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(fp.seen) != 1 || fp.seen[0] != "SELECT id FROM t WHERE id = $1" {
		t.Fatalf("rewritten = %v", fp.seen)
	}
}

func TestAdapterExec_success(t *testing.T) {
	t.Parallel()

	fp := &fakePool{tag: pgconn.NewCommandTag("INSERT 0 2")}
	a := &adapter{pool: fp}

	n, err := a.Exec(t.Context(), "INSERT INTO t VALUES (?)", "x")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, want 2", n)
	}
	if len(fp.seen) != 1 || fp.seen[0] != "INSERT INTO t VALUES ($1)" {
		t.Fatalf("rewritten = %v", fp.seen)
	}
}

func TestAdapterPing_success(t *testing.T) {
	t.Parallel()

	a := &adapter{pool: &fakePool{}}
	if err := a.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAdapterClose_closesPool(t *testing.T) {
	t.Parallel()

	fp := &fakePool{}
	a := &adapter{pool: fp}

	if err := a.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fp.closed {
		t.Fatal("want pool closed")
	}
}

func TestAdapterBeginTx_successPaths(t *testing.T) {
	t.Parallel()

	ft := &fakeTx{
		queryRows: &fakeRows{nextVals: []bool{true, false}},
		execTag:   pgconn.NewCommandTag("DELETE 1"),
	}
	fp := &fakePool{tx: ft}
	a := &adapter{pool: fp}

	tx, err := a.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	rows, err := tx.Query(t.Context(), "SELECT ?", "7")
	if err != nil {
		t.Fatalf("tx Query: %v", err)
	}
	if !rows.Next() {
		t.Fatal("want one row")
	}

	n, err := tx.Exec(t.Context(), "DELETE FROM t WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("tx Exec: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, want 1", n)
	}

	if err := tx.Savepoint(t.Context(), "sp1"); err != nil {
		t.Fatalf("Savepoint: %v", err)
	}
	if err := tx.RollbackTo(t.Context(), "sp1"); err != nil {
		t.Fatalf("RollbackTo: %v", err)
	}

	wired, ok := tx.(*pgTx)
	if !ok {
		t.Fatalf("want *pgTx, got %T", tx)
	}
	wired.conn = &fakeConn{}
	if err := tx.Ping(t.Context()); err != nil {
		t.Fatalf("tx Ping healthy: %v", err)
	}

	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestPgTxPing_connFailure(t *testing.T) {
	t.Parallel()

	bad := &pgTx{tx: &fakeTx{}, conn: &fakeConn{err: errors.New("conn down")}}
	if err := bad.Ping(t.Context()); err == nil {
		t.Fatal("want error for failed conn ping")
	}
}
