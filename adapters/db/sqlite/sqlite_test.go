package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
)

// NOTE: no registry test here by design. db.Register rejects duplicate
// adapter registration process-wide, so a Register+Open test in this
// package could collide with other adapter packages' tests in one binary;
// core covers registry mechanics with fakes. New is tested directly.

// newTempDB opens a file-backed adapter in a temp dir.
func newTempDB(t *testing.T, opts db.Options) db.DB {
	t.Helper()

	if opts.Path == "" {
		opts.Path = filepath.Join(t.TempDir(), "test.db")
	}

	d, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() {
		_ = d.Close(t.Context())
	})

	return d
}

// newMemoryDB opens an isolated :memory: adapter.
func newMemoryDB(t *testing.T) db.DB {
	t.Helper()

	d, err := New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() {
		_ = d.Close(t.Context())
	})

	return d
}

// mustExec fails the test on Exec error.
func mustExec(t *testing.T, d db.DB, query string, args ...any) int64 {
	t.Helper()

	n, err := d.Exec(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("Exec(%q): %v", query, err)
	}

	return n
}

// assertWrapped fails unless err is non-nil with a non-nil Unwrap chain
// (i.e. a %w-propagated driver failure, never a bare string).
func assertWrapped(t *testing.T, op string, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s: want wrapped error, got nil", op)
	}

	if errors.Unwrap(err) == nil {
		t.Fatalf("%s: want %%w chain, got %v", op, err)
	}
}

func pragmasOf(t *testing.T, dsn string) url.Values {
	t.Helper()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %q: %v", dsn, err)
	}

	return u.Query()
}

func pragmaSet(q url.Values, want string) bool {
	for _, v := range q["_pragma"] {
		if v == want {
			return true
		}
	}

	return false
}

func TestNew_DefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	d, err := New(db.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = d.Close(t.Context()) }()

	if err := d.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "app.db")); err != nil {
		t.Fatalf("want app.db created, got %v", err)
	}
}

func TestNew_WhitespacePath(t *testing.T) {
	if _, err := New(db.Options{Path: "   "}); err == nil {
		t.Fatal("want error for whitespace path, got nil")
	}
}

func TestNew_OpenError(t *testing.T) {
	old := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errStubOpen }
	t.Cleanup(func() { sqlOpen = old })

	_, err := New(db.Options{Path: ":memory:"})
	assertWrapped(t, "New", err)
}

func TestNew_ConfigureError(t *testing.T) {
	// Missing parent dir: file cannot be created, ping must fail and no
	// Path/DSN content may leak (asserted by absence, not string match).
	_, err := New(db.Options{Path: filepath.Join(t.TempDir(), "nope", "x.db")})
	assertWrapped(t, "New", err)
}

func TestNew_FilePoolKnobs(t *testing.T) {
	d := newTempDB(t, db.Options{
		Path:            filepath.Join(t.TempDir(), "pool.db"),
		MaxConns:        7,
		MinConns:        3,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: time.Second,
	})

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	if got := a.conn.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("MaxOpenConnections = %d, want 7", got)
	}
}

func TestNew_MemoryOverridesMaxConns(t *testing.T) {
	d, err := New(db.Options{Path: ":memory:", MaxConns: 10})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = d.Close(t.Context()) })

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	if got := a.conn.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1 (forced for :memory:)", got)
	}
}

func TestDialect(t *testing.T) {
	if got := newMemoryDB(t).Dialect(); got != "sqlite" {
		t.Fatalf("Dialect = %q, want sqlite", got)
	}
}

func TestExecCRUD(t *testing.T) {
	d := newTempDB(t, db.Options{})

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	mustExec(t, d, `INSERT INTO users (name) VALUES (?)`, "alice")
	mustExec(t, d, `UPDATE users SET name = ? WHERE name = ?`, "alice2", "alice")
	mustExec(t, d, `DELETE FROM users WHERE name = ?`, "alice2")
}

func TestExec_RowsAffected(t *testing.T) {
	d := newTempDB(t, db.Options{})

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	if n := mustExec(t, d, `INSERT INTO users (name) VALUES (?), (?), (?)`, "a", "b", "c"); n != 3 {
		t.Fatalf("insert affected = %d, want 3", n)
	}

	if n := mustExec(t, d, `UPDATE users SET name = ? WHERE name IN (?, ?)`, "z", "a", "b"); n != 2 {
		t.Fatalf("update affected = %d, want 2", n)
	}

	if n := mustExec(t, d, `UPDATE users SET name = ? WHERE name = ?`, "q", "nonexistent"); n != 0 {
		t.Fatalf("no-op update affected = %d, want 0", n)
	}

	if n := mustExec(t, d, `DELETE FROM users WHERE name = ?`, "z"); n != 2 {
		t.Fatalf("delete affected = %d, want 2", n)
	}

	if n := mustExec(t, d, `DELETE FROM users WHERE name = ?`, "nonexistent"); n != 0 {
		t.Fatalf("no-op delete affected = %d, want 0", n)
	}
}

func TestExec_BadSQL(t *testing.T) {
	_, err := newMemoryDB(t).Exec(t.Context(), `INSERT INTO nope (x) VALUES (1)`)
	assertWrapped(t, "Exec", err)
}

func TestExec_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := newMemoryDB(t).Exec(ctx, `SELECT 1`)
	assertWrapped(t, "Exec", err)
}

func TestQuery_ScanColumnsErr(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	mustExec(t, d, `INSERT INTO users (name) VALUES (?), (?)`, "alice", "bob")

	rows, err := d.Query(ctx, `SELECT id, name FROM users WHERE name = ? ORDER BY id`, "alice")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}

	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("Columns = %v, want [id name]", cols)
	}

	var got []string

	for rows.Next() {
		var (
			id   int
			name string
		)

		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		got = append(got, name)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	if len(got) != 1 || got[0] != "alice" {
		t.Fatalf("got %v, want [alice]", got)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestQuery_NoRows(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	rows, err := d.Query(ctx, `SELECT name FROM users WHERE id > 0`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		t.Fatal("want no rows, got one")
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestQuery_BadSQL(t *testing.T) {
	_, err := newMemoryDB(t).Query(t.Context(), `SELECT * FROM nope`)
	assertWrapped(t, "Query", err)
}

func TestQuery_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := newMemoryDB(t).Query(ctx, `SELECT 1`)
	assertWrapped(t, "Query", err)
}

func TestColumns_ClosedRows(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	rows, err := d.Query(ctx, `SELECT id FROM t`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := rows.Columns(); err == nil {
		t.Fatal("want Columns error on closed rows, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Fatalf("want %%w chain, got %v", err)
	}
}

func TestPing(t *testing.T) {
	if err := newMemoryDB(t).Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose_PingAfterClose(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	if err := d.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	assertWrapped(t, "Ping", d.Ping(ctx))
}

func TestClose_CanceledContext(t *testing.T) {
	d := newTempDB(t, db.Options{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := d.Close(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Close = %v, want context.Canceled", err)
	}
}

func TestClose_StmtError(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	// Poison the cache with a stickyErr stmt: Tx.Stmt on a finished tx
	// returns one, and its Close reports the sticky error.
	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	// Prepare before BeginTx: :memory: forces a single connection and the
	// open tx would otherwise hold it, deadlocking PrepareContext.
	raw, err := a.conn.PrepareContext(ctx, `SELECT 1`)
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	//nolint:forcetypeassert // test-only access to concrete tx for Stmt poisoning
	poisoned := tx.(*sqliteTx).tx.StmtContext(ctx, raw)
	_ = raw.Close() //nolint:sqlclosecheck // deliberate immediate close to poison `poisoned` before Close(ctx) runs below, not end-of-test cleanup
	a.stmtCache.Put("poison", poisoned)

	if err := d.Close(ctx); err == nil {
		t.Fatal("want Close error from poisoned stmt, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Fatalf("want %%w chain, got %v", err)
	}
}

func TestAdapter_OnClosedPool(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	if err := d.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := d.Query(ctx, `SELECT 1`); err == nil {
		t.Error("Query on closed pool: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("Query on closed pool: want %w chain")
	}

	if _, err := d.Exec(ctx, `SELECT 1`); err == nil {
		t.Error("Exec on closed pool: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("Exec on closed pool: want %w chain")
	}

	assertWrapped(t, "Ping", d.Ping(ctx))

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	if _, err := tr.BeginTx(ctx, nil); err == nil {
		t.Error("BeginTx on closed pool: want error, got nil")
	}

	p, ok := d.(db.Preparer)
	if !ok {
		t.Fatalf("%T does not implement db.Preparer", d)
	}

	if _, err := p.Prepare(ctx, `SELECT 1`); err == nil {
		t.Error("Prepare on closed pool: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("Prepare on closed pool: want %w chain")
	}
}

func TestBeginTx_NilOptsAndLevels(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	levels := []db.IsolationLevel{
		db.ReadCommitted,
		db.RepeatableRead,
		db.Serializable,
		db.IsolationLevel(99), // unknown maps to read committed
	}

	for _, lvl := range levels {
		tx, err := tr.BeginTx(ctx, &db.TxOptions{Isolation: lvl})
		if err != nil {
			t.Fatalf("BeginTx(%v): %v", lvl, err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO t (v) VALUES (?)`, lvl.String()); err != nil {
			t.Fatalf("tx Exec(%v): %v", lvl, err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit(%v): %v", lvl, err)
		}
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(nil): %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT count(*) FROM t`)
	if err != nil {
		t.Fatalf("tx Query: %v", err)
	}

	var count int

	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			t.Fatalf("Scan: %v", err)
		}
	}

	_ = rows.Close()

	if count != len(levels) {
		t.Fatalf("count = %d, want %d", count, len(levels))
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestTx_CommitRollback(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users (name) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("tx Exec: %v", err)
	}

	err = tx.Ping(ctx)
	if err != nil {
		t.Fatalf("tx Ping while open: %v", err)
	}

	if got := tx.Dialect(); got != "sqlite" {
		t.Fatalf("tx Dialect = %q, want sqlite", got)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	err = tx.Ping(ctx)
	if err == nil {
		t.Fatal("tx Ping after commit: want error, got nil")
	}

	err = tx.Close(ctx)
	if err != nil {
		t.Fatalf("tx Close after commit: %v", err)
	}

	rows, err := d.Query(ctx, `SELECT name FROM users`)
	if err != nil {
		t.Fatalf("Query after commit: %v", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatal("want row after commit")
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTx_DoubleCommit(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	err = tx.Commit(ctx)
	if err == nil {
		t.Fatal("second Commit: want error, got nil")
	}

	var txErr *db.TxError
	if !errors.As(err, &txErr) || txErr.Op != "commit" {
		t.Fatalf("second Commit = %v, want *db.TxError{Op: commit}", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback after commit should be nil, got %v", err)
	}
}

func TestTx_RollbackIdempotent(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("first Rollback: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("second Rollback: %v", err)
	}

	if err := tx.Commit(ctx); err == nil {
		t.Fatal("Commit after rollback: want error, got nil")
	}
}

func TestTx_CommitDriverError(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	//nolint:forcetypeassert // test-only access to inner tx
	inner := tx.(*sqliteTx).tx
	err = inner.Rollback()
	if err != nil {
		t.Fatalf("inner Rollback: %v", err)
	}

	err = tx.Commit(ctx)
	if err == nil {
		t.Fatal("Commit after inner rollback: want error, got nil")
	}

	var txErr *db.TxError
	if !errors.As(err, &txErr) || txErr.Op != "commit" {
		t.Fatalf("Commit = %v, want *db.TxError{Op: commit}", err)
	}
}

func TestTx_RollbackDriverError(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	//nolint:forcetypeassert // test-only access to inner tx
	inner := tx.(*sqliteTx).tx
	if err := inner.Rollback(); err != nil {
		t.Fatalf("inner Rollback: %v", err)
	}

	assertWrapped(t, "Rollback", tx.Rollback(ctx))
}

func TestTx_ClosePropagatesRollbackError(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	//nolint:forcetypeassert // test-only access to inner tx
	if err := tx.(*sqliteTx).tx.Rollback(); err != nil {
		t.Fatalf("inner Rollback: %v", err)
	}

	if err := tx.Close(ctx); err == nil {
		t.Fatal("Close with failing rollback: want error, got nil")
	}
}

func TestTx_CloseRollsBack(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users (name) VALUES (?)`, "ghost")
	if err != nil {
		t.Fatalf("tx Exec: %v", err)
	}

	err = tx.Close(ctx)
	if err != nil {
		t.Fatalf("tx Close: %v", err)
	}

	rows, err := d.Query(ctx, `SELECT name FROM users WHERE name = ?`, "ghost")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		t.Fatal("want no row after tx Close, got one")
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTx_BadQueryExec(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Query(ctx, `SELECT * FROM nope`); err == nil {
		t.Error("tx Query bad SQL: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("tx Query bad SQL: want %w chain")
	}

	if _, err := tx.Exec(ctx, `INSERT INTO nope (x) VALUES (1)`); err == nil {
		t.Error("tx Exec bad SQL: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("tx Exec bad SQL: want %w chain")
	}
}

func TestWithTx_SuccessAndRollback(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	err := db.WithTx(ctx, d, &db.TxOptions{Isolation: db.Serializable}, func(txCtx context.Context, tx db.Tx) error {
		if _, ok := db.TxFromContext(txCtx); !ok {
			t.Error("tx not in context inside WithTx")
		}

		_, execErr := tx.Exec(txCtx, `INSERT INTO users (name) VALUES (?)`, "carol")

		return execErr
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	wantErr := errors.New("fn error")

	err = db.WithTx(ctx, d, nil, func(txCtx context.Context, tx db.Tx) error {
		_, execErr := tx.Exec(txCtx, `INSERT INTO users (name) VALUES (?)`, "dave")
		if execErr != nil {
			return execErr
		}

		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx = %v, want %v", err, wantErr)
	}

	rows, err := d.Query(ctx, `SELECT name FROM users ORDER BY name`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var got []string

	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		got = append(got, n)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(got) != 1 || got[0] != "carol" {
		t.Fatalf("got %v, want [carol]", got)
	}
}

func TestSavepoint_RollbackTo(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users (name) VALUES (?)`, "a")
	if err != nil {
		t.Fatalf("insert a: %v", err)
	}

	err = tx.Savepoint(ctx, "sp1")
	if err != nil {
		t.Fatalf("Savepoint: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO users (name) VALUES (?)`, "b")
	if err != nil {
		t.Fatalf("insert b: %v", err)
	}

	err = tx.RollbackTo(ctx, "sp1")
	if err != nil {
		t.Fatalf("RollbackTo: %v", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := d.Query(ctx, `SELECT name FROM users ORDER BY name`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var got []string

	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("Scan: %v", err)
		}

		got = append(got, n)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("got %v, want [a]", got)
	}
}

func TestSavepoint_InvalidNames(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	bad := []string{"", "bad-name", "123bad", "has space", "semi;colon", `"quoted"`}
	for _, name := range bad {
		if err := tx.Savepoint(ctx, name); err == nil {
			t.Errorf("Savepoint(%q): want error, got nil", name)
		}

		if err := tx.RollbackTo(ctx, name); err == nil {
			t.Errorf("RollbackTo(%q): want error, got nil", name)
		}
	}

	for _, name := range []string{"sp1", "_sp", "SP_2x"} {
		if err := tx.Savepoint(ctx, name); err != nil {
			t.Errorf("Savepoint(%q): %v", name, err)
		}
	}
}

func TestSavepoint_AfterCommit(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)

	tr, ok := d.(db.Transactor)
	if !ok {
		t.Fatalf("%T does not implement db.Transactor", d)
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	assertWrapped(t, "Savepoint", tx.Savepoint(ctx, "sp1"))
	assertWrapped(t, "RollbackTo", tx.RollbackTo(ctx, "sp1"))
}

func TestPrepare_CacheHit(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)

	p, ok := d.(db.Preparer)
	if !ok {
		t.Fatalf("%T does not implement db.Preparer", d)
	}

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	const query = `INSERT INTO t (v) VALUES (?)`

	for i := 0; i < 3; i++ {
		stmt, err := p.Prepare(ctx, query)
		if err != nil {
			t.Fatalf("Prepare %d: %v", i, err)
		}

		n, err := stmt.Exec(ctx, fmt.Sprintf("v%d", i))
		if err != nil {
			t.Fatalf("Exec %d: %v", i, err)
		}

		if n != 1 {
			t.Fatalf("Exec %d affected = %d, want 1", i, n)
		}

		if err := stmt.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}

	if got := a.stmtCache.Len(); got != 1 {
		t.Fatalf("cache Len = %d, want 1", got)
	}

	stmt, err := p.Prepare(ctx, `SELECT v FROM t ORDER BY id`)
	if err != nil {
		t.Fatalf("Prepare select: %v", err)
	}
	defer func() { _ = stmt.Close() }()

	rows, err := stmt.Query(ctx)
	if err != nil {
		t.Fatalf("stmt Query: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var count int

	for rows.Next() {
		count++
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows Err: %v", err)
	}

	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestPrepare_BadSQL(t *testing.T) {
	p, ok := newMemoryDB(t).(db.Preparer)
	if !ok {
		t.Fatal("adapter does not implement db.Preparer")
	}

	_, err := p.Prepare(t.Context(), `SELECT * FROM nope WHERE (`)
	assertWrapped(t, "Prepare", err)
}

func TestPrepare_ConcurrentSameQuery(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)
	mustExec(t, d, `INSERT INTO t (v) VALUES (?), (?)`, "a", "b")

	p, ok := d.(db.Preparer)
	if !ok {
		t.Fatalf("%T does not implement db.Preparer", d)
	}

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	const goroutines = 16

	errCh := make(chan error, goroutines)

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()

			stmt, err := p.Prepare(ctx, `SELECT v FROM t ORDER BY id`)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d Prepare: %w", i, err)

				return
			}

			rows, err := stmt.Query(ctx)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d Query: %w", i, err)

				return
			}

			var got int

			for rows.Next() {
				got++
			}

			if err := rows.Err(); err != nil {
				errCh <- fmt.Errorf("goroutine %d rows.Err: %w", i, err)
			}

			if err := rows.Close(); err != nil {
				errCh <- fmt.Errorf("goroutine %d close rows: %w", i, err)
			}

			if got != 2 {
				errCh <- fmt.Errorf("goroutine %d: scanned %d rows, want 2", i, got)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	if got := a.stmtCache.Len(); got != 1 {
		t.Fatalf("cache Len = %d, want 1", got)
	}
}

func TestStmt_QueryExecAfterDrop(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)

	p, ok := d.(db.Preparer)
	if !ok {
		t.Fatalf("%T does not implement db.Preparer", d)
	}

	sel, err := p.Prepare(ctx, `SELECT v FROM t`)
	if err != nil {
		t.Fatalf("Prepare select: %v", err)
	}
	defer func() { _ = sel.Close() }()

	ins, err := p.Prepare(ctx, `INSERT INTO t (v) VALUES (?)`)
	if err != nil {
		t.Fatalf("Prepare insert: %v", err)
	}
	defer func() { _ = ins.Close() }()

	mustExec(t, d, `DROP TABLE t`)

	if _, err := sel.Query(ctx); err == nil {
		t.Error("stmt Query after drop: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("stmt Query after drop: want %w chain")
	}

	if _, err := ins.Exec(ctx, "x"); err == nil {
		t.Error("stmt Exec after drop: want error, got nil")
	} else if errors.Unwrap(err) == nil {
		t.Error("stmt Exec after drop: want %w chain")
	}
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantPrefix string
		check      func(t *testing.T, q url.Values)
	}{
		{
			name:       "memory",
			path:       ":memory:",
			wantPrefix: "file::memory:?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("cache") != "shared" {
					t.Errorf("cache = %q, want shared", q.Get("cache"))
				}

				if !pragmaSet(q, pragmaFK) || !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing default pragmas in %v", q)
				}
			},
		},
		{
			name:       "file memory form",
			path:       "file::memory:",
			wantPrefix: "file::memory:?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("cache") != "shared" {
					t.Errorf("cache = %q, want shared", q.Get("cache"))
				}
			},
		},
		{
			name:       "file memory preserves query and cache",
			path:       "file::memory:?mode=memory",
			wantPrefix: "file::memory:?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("mode") != "memory" {
					t.Errorf("mode = %q, want memory", q.Get("mode"))
				}

				if q.Get("cache") != "shared" {
					t.Errorf("cache = %q, want shared", q.Get("cache"))
				}
			},
		},
		{
			name:       "file memory user cache wins",
			path:       "file::memory:?cache=private",
			wantPrefix: "file::memory:?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("cache") != "private" {
					t.Errorf("cache = %q, want private", q.Get("cache"))
				}
			},
		},
		{
			name:       "file adds pragmas",
			path:       "app.db",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Has("cache") {
					t.Errorf("file DSN must not add cache, got %q", q.Get("cache"))
				}

				if !pragmaSet(q, pragmaFK) || !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing default pragmas in %v", q)
				}
			},
		},
		{
			name:       "file preserves user pragma",
			path:       "app.db?_pragma=journal_mode(WAL)",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				for _, want := range []string{"journal_mode(WAL)", pragmaFK, pragmaBusy} {
					if !pragmaSet(q, want) {
						t.Errorf("missing %q in %v", want, q)
					}
				}
			},
		},
		{
			name:       "bare pragma without parens preserved",
			path:       "app.db?_pragma=wal",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if !pragmaSet(q, "wal") {
					t.Errorf("missing wal in %v", q)
				}

				if !pragmaSet(q, pragmaFK) || !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing default pragmas in %v", q)
				}
			},
		},
		{
			name:       "unparsable query still gets pragmas",
			path:       "app.db?;",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if !pragmaSet(q, pragmaFK) || !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing default pragmas in %v", q)
				}
			},
		},
		{
			name:       "foreign keys override",
			path:       "app.db?_foreign_keys=0",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("_foreign_keys") != "0" {
					t.Errorf("_foreign_keys = %q, want 0", q.Get("_foreign_keys"))
				}

				for _, v := range q["_pragma"] {
					if strings.Contains(v, "foreign_keys") {
						t.Errorf("must not add conflicting pragma, got %q", v)
					}
				}

				if !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing %q", pragmaBusy)
				}
			},
		},
		{
			name:       "fk alias override",
			path:       "app.db?_fk=0",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				for _, v := range q["_pragma"] {
					if strings.Contains(v, "foreign_keys") {
						t.Errorf("must not add conflicting pragma, got %q", v)
					}
				}
			},
		},
		{
			name:       "timeout alias override",
			path:       "app.db?_timeout=1000",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				for _, v := range q["_pragma"] {
					if strings.Contains(v, "busy_timeout") {
						t.Errorf("must not add conflicting pragma, got %q", v)
					}
				}

				if !pragmaSet(q, pragmaFK) {
					t.Errorf("missing %q", pragmaFK)
				}
			},
		},
		{
			name:       "file scheme keeps mode and user pragma",
			path:       "file:app.db?mode=rwc&_pragma=busy_timeout(1000)",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if q.Get("mode") != "rwc" {
					t.Errorf("mode = %q, want rwc", q.Get("mode"))
				}

				if !pragmaSet(q, "busy_timeout(1000)") {
					t.Error("missing user busy_timeout")
				}

				if !pragmaSet(q, pragmaFK) {
					t.Errorf("missing %q", pragmaFK)
				}

				if pragmaSet(q, pragmaBusy) {
					t.Errorf("must keep user busy_timeout, got default %q", pragmaBusy)
				}
			},
		},
		{
			name:       "file scheme case-insensitive pragma match",
			path:       "file:app.db?_pragma=BUSY_TIMEOUT(1000)",
			wantPrefix: "file:app.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if pragmaSet(q, pragmaBusy) {
					t.Errorf("must keep user busy_timeout, got default %q", pragmaBusy)
				}
			},
		},
		{
			name:       "file URI scheme",
			path:       "file:///tmp/x.db",
			wantPrefix: "file:///tmp/x.db?",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				if !pragmaSet(q, pragmaFK) || !pragmaSet(q, pragmaBusy) {
					t.Errorf("missing default pragmas in %v", q)
				}
			},
		},
		{
			name:       "space escaped",
			path:       "/tmp/a b.db",
			wantPrefix: "file:/tmp/a%20b.db?",
			check:      nil,
		},
		{
			name:       "colon path not a scheme",
			path:       "log:2026.db",
			wantPrefix: "file:",
			check: func(t *testing.T, q url.Values) {
				t.Helper()

				_ = q
			},
		},
		{
			name:       "hash escaped in path",
			path:       "file:/tmp/a#b.db",
			wantPrefix: "file:/tmp/a%23b.db?",
			check:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDSN(tt.path)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("buildDSN(%q) = %q, want prefix %q", tt.path, got, tt.wantPrefix)
			}

			if strings.Contains(got, "#") {
				t.Fatalf("buildDSN(%q) = %q contains raw #", tt.path, got)
			}

			if tt.check != nil {
				tt.check(t, pragmasOf(t, got))
			}
		})
	}
}

func TestBuildDSN_HashFileCreated(t *testing.T) {
	for _, suffix := range []string{"", "?mode=rwc"} {
		name := "web#1.db"
		dir := t.TempDir()

		d, err := New(db.Options{Path: filepath.Join(dir, name) + suffix})
		if err != nil {
			t.Fatalf("New %q: %v", name+suffix, err)
		}

		t.Cleanup(func() { _ = d.Close(t.Context()) })

		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("want file %q, got %v", name, err)
		}

		if _, err := os.Stat(filepath.Join(dir, "web")); !os.IsNotExist(err) {
			t.Fatalf("stray file %q created", "web")
		}
	}
}

func TestBuildDSN_PercentFileCreated(t *testing.T) {
	for _, name := range []string{"a%b.db", "a%20b.db"} {
		dir := t.TempDir()

		d, err := New(db.Options{Path: "file:" + filepath.Join(dir, name)})
		if err != nil {
			t.Fatalf("New %q: %v", name, err)
		}

		t.Cleanup(func() { _ = d.Close(t.Context()) })

		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("want file %q, got %v", name, err)
		}

		mustExec(t, d, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	}
}

func TestPragmas_OnConnection(t *testing.T) {
	d := newTempDB(t, db.Options{})

	a, ok := d.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", d)
	}

	var fk, timeout int

	if err := a.conn.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys pragma: %v", err)
	}

	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	if err := a.conn.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatalf("busy_timeout pragma: %v", err)
	}

	if timeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestFKEnforced(t *testing.T) {
	d := newTempDB(t, db.Options{})
	ctx := t.Context()

	mustExec(t, d, `CREATE TABLE parent (id INTEGER PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id))`)
	mustExec(t, d, `INSERT INTO parent (id) VALUES (1)`)
	mustExec(t, d, `INSERT INTO child (parent_id) VALUES (1)`)

	if _, err := d.Exec(ctx, `INSERT INTO child (parent_id) VALUES (99)`); err == nil {
		t.Fatal("want FK violation, got nil")
	}
}

func TestMemoryDB_ConcurrentUse(t *testing.T) {
	d := newMemoryDB(t)
	ctx := t.Context()

	const n = 16

	errs := make([]error, n)

	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			if _, err := d.Exec(ctx, `CREATE TABLE IF NOT EXISTS mem (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
				errs[i] = fmt.Errorf("create: %w", err)

				return
			}

			if _, err := d.Exec(ctx, `INSERT INTO mem (v) VALUES (?)`, fmt.Sprintf("v%d", i)); err != nil {
				errs[i] = fmt.Errorf("insert: %w", err)

				return
			}

			rows, err := d.Query(ctx, `SELECT count(*) FROM mem`)
			if err != nil {
				errs[i] = fmt.Errorf("query: %w", err)

				return
			}
			defer func() { _ = rows.Close() }()

			if !rows.Next() {
				errs[i] = errors.New("no rows")

				return
			}

			var count int
			if err := rows.Scan(&count); err != nil {
				errs[i] = fmt.Errorf("scan: %w", err)

				return
			}

			if count < 1 {
				errs[i] = fmt.Errorf("count = %d, want >= 1", count)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
}

// --- Stub driver: covers defensive branches the real driver never takes ---
// modernc's Result.RowsAffected never errors and *sql.DB.Close is
// idempotent, so these branches need a fake connector.

var (
	errStubOpen      = errors.New("stub: open failed")
	errStubRows      = errors.New("stub: rows affected unavailable")
	errStubConnClose = errors.New("stub: connector close failed")
)

type stubResult struct{}

func (stubResult) LastInsertId() (int64, error) { return 0, nil }
func (stubResult) RowsAffected() (int64, error) { return 0, errStubRows }

type stubStmt struct{}

func (stubStmt) Close() error                               { return nil }
func (stubStmt) NumInput() int                              { return -1 }
func (stubStmt) Exec([]driver.Value) (driver.Result, error) { return stubResult{}, nil }
func (stubStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("stub: query unsupported")
}

type stubConn struct{}

func (stubConn) Prepare(string) (driver.Stmt, error) { return stubStmt{}, nil }
func (stubConn) Close() error                        { return nil }
func (stubConn) Begin() (driver.Tx, error)           { return stubTx{}, nil }

// BeginTx satisfies driver.ConnBeginTx: the adapter always passes an
// explicit isolation level, which database/sql rejects without it.
func (stubConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return stubTx{}, nil
}

type stubTx struct{}

func (stubTx) Commit() error   { return nil }
func (stubTx) Rollback() error { return nil }

type stubConnector struct{ closeErr error }

func (stubConnector) Connect(context.Context) (driver.Conn, error) { return stubConn{}, nil }
func (stubConnector) Driver() driver.Driver                        { return nil }
func (c stubConnector) Close() error                               { return c.closeErr }

func newStubAdapter() *adapter {
	return &adapter{conn: sql.OpenDB(stubConnector{}), stmtCache: newStmtCache(4)}
}

func TestStubAdapter_ExecRowsAffectedError(t *testing.T) {
	a := newStubAdapter()
	t.Cleanup(func() { _ = a.conn.Close() })

	_, err := a.Exec(t.Context(), `INSERT INTO t (v) VALUES (?)`, "x")
	if !errors.Is(err, errStubRows) {
		t.Fatalf("Exec = %v, want %v", err, errStubRows)
	}
}

func TestStubAdapter_StmtRowsAffectedError(t *testing.T) {
	a := newStubAdapter()
	t.Cleanup(func() { _ = a.conn.Close() })

	ctx := t.Context()

	stmt, err := a.Prepare(ctx, `INSERT INTO t (v) VALUES (?)`)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer func() { _ = stmt.Close() }()

	if _, err := stmt.Exec(ctx, "x"); !errors.Is(err, errStubRows) {
		t.Fatalf("stmt Exec = %v, want %v", err, errStubRows)
	}
}

func TestStubAdapter_TxRowsAffectedError(t *testing.T) {
	a := newStubAdapter()
	t.Cleanup(func() { _ = a.conn.Close() })

	ctx := t.Context()

	tx, err := a.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `INSERT INTO t (v) VALUES (?)`, "x"); !errors.Is(err, errStubRows) {
		t.Fatalf("tx Exec = %v, want %v", err, errStubRows)
	}
}

func TestStubAdapter_CloseConnError(t *testing.T) {
	a := &adapter{conn: sql.OpenDB(stubConnector{closeErr: errStubConnClose}), stmtCache: newStmtCache(4)}

	if err := a.Close(t.Context()); !errors.Is(err, errStubConnClose) {
		t.Fatalf("Close = %v, want %v", err, errStubConnClose)
	}
}
