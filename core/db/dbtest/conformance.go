// Package dbtest provides the conformance kit third-party db adapters run to prove backend parity.
package dbtest

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// conformanceTable is the scratch table the kit creates. Cover tests run it
// against a live Postgres when POSTGRES_DSN is set, so the name is distinctive
// and the Close subtest drops it best-effort to leave shared databases clean.
const conformanceTable = "zever_conformance_kv"

// Conformance verifies factory-built databases implement the db.DB contract:
// Dialect reporting, Ping, Exec/Query round-trip with the portable Rows
// surface (Columns, Next, Scan, Err, Close), %w-wrapped driver errors,
// WithTx commit and rollback, nested-transaction rejection, optional Preparer
// support, and Close. Each subtest takes a fresh instance from factory so
// cases stay isolated. Tests are deterministic and touch no network.
func Conformance(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	t.Run("Dialect", func(t *testing.T) { conformanceDialect(t, factory) })
	t.Run("Ping", func(t *testing.T) { conformancePing(t, factory) })
	t.Run("ExecQuery", func(t *testing.T) { conformanceExecQuery(t, factory) })
	t.Run("QueryError", func(t *testing.T) { conformanceQueryError(t, factory) })
	t.Run("ExecError", func(t *testing.T) { conformanceExecError(t, factory) })
	t.Run("WithTxCommit", func(t *testing.T) { conformanceWithTxCommit(t, factory) })
	t.Run("WithTxRollback", func(t *testing.T) { conformanceWithTxRollback(t, factory) })
	t.Run("WithTxNested", func(t *testing.T) { conformanceWithTxNested(t, factory) })
	t.Run("Preparer", func(t *testing.T) { conformancePreparer(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceDialect(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	d := factory(t)

	if got := d.Dialect(); got == "" {
		t.Error("Dialect() = empty, want non-empty registration name")
	}
}

func conformancePing(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	if err := factory(t).Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

// mustScratchTable creates and empties the scratch table. DDL and ? placeholders
// are portable: the postgres adapter rewrites ? to $n.
func mustScratchTable(t *testing.T, d db.DB) {
	t.Helper()

	ctx := t.Context()

	if _, err := d.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+conformanceTable+` (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("Exec(create) error = %v", err)
	}

	if _, err := d.Exec(ctx, `DELETE FROM `+conformanceTable); err != nil {
		t.Fatalf("Exec(clear) error = %v", err)
	}
}

func conformanceExecQuery(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)
	mustScratchTable(t, d)

	if n, err := d.Exec(ctx, `INSERT INTO `+conformanceTable+` (k, v) VALUES (?, ?), (?, ?)`, "a", "a", "b", "b"); err != nil {
		t.Fatalf("Exec(insert) error = %v", err)
	} else if n != 2 {
		t.Fatalf("Exec(insert) = %d, want 2", n)
	}

	rows, err := d.Query(ctx, `SELECT k, v FROM `+conformanceTable+` ORDER BY k`)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("Rows.Close() error = %v", closeErr)
		}
	}()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns() error = %v", err)
	}

	if len(cols) != 2 {
		t.Fatalf("Columns() = %q, want 2 columns", cols)
	}

	var got []string

	for rows.Next() {
		var k, v string

		if err := rows.Scan(&k, &v); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}

		got = append(got, k+"="+v)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("Rows.Err() = %v", err)
	}

	if len(got) != 2 || got[0] != "a=a" || got[1] != "b=b" {
		t.Fatalf("rows = %q, want [a=a b=b]", got)
	}
}

func conformanceQueryError(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	rows, err := factory(t).Query(t.Context(), `SELECT * FROM zever_conformance_no_such_table_xyz`)
	if err == nil {
		_ = rows.Close()

		t.Fatal("Query(bad table) = nil, want wrapped driver error")
	}

	if errors.Unwrap(err) == nil {
		t.Errorf("Query(bad table) err = %v, want %%w chain", err)
	}
}

func conformanceExecError(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	if _, err := factory(t).Exec(t.Context(), `INSERT INTO zever_conformance_no_such_table_xyz (k) VALUES (?)`, "a"); err == nil {
		t.Fatal("Exec(bad table) = nil, want wrapped driver error")
	} else if errors.Unwrap(err) == nil {
		t.Errorf("Exec(bad table) err = %v, want %%w chain", err)
	}
}

// transactorOf reports whether d supports transactions. Adapters without
// Transactor fail WithTx with ErrTxUnsupported; the kit asserts that instead
// of the commit path so third-party non-transactional backends still conform.
func transactorOf(d db.DB) bool {
	_, ok := d.(db.Transactor)

	return ok
}

func conformanceWithTxCommit(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)

	if !transactorOf(d) {
		if err := db.WithTx(ctx, d, nil, func(context.Context, db.Tx) error { return nil }); !errors.Is(err, db.ErrTxUnsupported) {
			t.Fatalf("WithTx() err = %v, want ErrTxUnsupported", err)
		}

		t.Skip("adapter does not implement Transactor")
	}

	mustScratchTable(t, d)

	if err := db.WithTx(ctx, d, nil, func(txCtx context.Context, tx db.Tx) error {
		if _, err := tx.Exec(txCtx, `INSERT INTO `+conformanceTable+` (k, v) VALUES (?, ?)`, "tx1", "tx1"); err != nil {
			return err
		}

		return nil
	}); err != nil {
		t.Fatalf("WithTx(commit) error = %v", err)
	}

	rows, err := d.Query(ctx, `SELECT v FROM `+conformanceTable+` WHERE k = ?`, "tx1")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatal("committed row tx1 missing")
	}

	var v string

	if err := rows.Scan(&v); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if v != "tx1" {
		t.Errorf("committed v = %q, want tx1", v)
	}
}

func conformanceWithTxRollback(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)

	if !transactorOf(d) {
		t.Skip("adapter does not implement Transactor")
	}

	mustScratchTable(t, d)

	boom := errors.New("dbtest: rollback probe")

	err := db.WithTx(ctx, d, nil, func(txCtx context.Context, tx db.Tx) error {
		if _, err := tx.Exec(txCtx, `INSERT INTO `+conformanceTable+` (k, v) VALUES (?, ?)`, "txrb", "txrb"); err != nil {
			return err
		}

		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithTx() err = %v, want rollback probe", err)
	}

	rows, err := d.Query(ctx, `SELECT v FROM `+conformanceTable+` WHERE k = ?`, "txrb")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	defer func() { _ = rows.Close() }()

	if rows.Next() {
		t.Error("rolled-back row txrb visible, want absent")
	}
}

func conformanceWithTxNested(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)

	if !transactorOf(d) {
		t.Skip("adapter does not implement Transactor")
	}

	mustScratchTable(t, d)

	err := db.WithTx(ctx, d, nil, func(txCtx context.Context, tx db.Tx) error {
		return db.WithTx(txCtx, tx, nil, func(context.Context, db.Tx) error { return nil })
	})
	if !errors.Is(err, db.ErrNestedTx) {
		t.Errorf("WithTx(nested) err = %v, want ErrNestedTx", err)
	}
}

func conformancePreparer(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)

	p, ok := d.(db.Preparer)
	if !ok {
		t.Skip("adapter does not implement Preparer")
	}

	mustScratchTable(t, d)

	const insert = `INSERT INTO ` + conformanceTable + ` (k, v) VALUES (?, ?)`

	s1, err := p.Prepare(ctx, insert)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	defer func() { _ = s1.Close() }()

	s2, err := p.Prepare(ctx, insert)
	if err != nil {
		t.Fatalf("Prepare(same text) error = %v", err)
	}

	defer func() { _ = s2.Close() }()

	if n, execErr := s1.Exec(ctx, "p1", "v1"); execErr != nil {
		t.Fatalf("Stmt.Exec() error = %v", execErr)
	} else if n != 1 {
		t.Errorf("Stmt.Exec() = %d, want 1", n)
	}

	const selectByK = `SELECT v FROM ` + conformanceTable + ` WHERE k = ?`

	q, err := p.Prepare(ctx, selectByK)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	defer func() { _ = q.Close() }()

	rows, err := q.Query(ctx, "p1")
	if err != nil {
		t.Fatalf("Stmt.Query() error = %v", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatal("prepared-query row p1 missing")
	}

	var v string

	if err := rows.Scan(&v); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if v != "v1" {
		t.Errorf("prepared-query v = %q, want v1", v)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) db.DB) {
	t.Helper()

	ctx := t.Context()
	d := factory(t)

	// Best-effort scratch cleanup for shared live databases; sqlite :memory:
	// instances vanish with Close either way.
	_, _ = d.Exec(ctx, `DROP TABLE IF EXISTS `+conformanceTable)

	if err := d.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := d.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if _, err := d.Query(ctx, `SELECT 1`); err == nil {
		t.Error("Query after Close = nil, want error")
	}

	if _, err := d.Exec(ctx, `SELECT 1`); err == nil {
		t.Error("Exec after Close = nil, want error")
	}

	if err := d.Ping(ctx); err == nil {
		t.Error("Ping after Close = nil, want error")
	}
}
