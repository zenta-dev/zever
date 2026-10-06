package cdc

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/zenta-dev/zever/core/db"
)

// postgresDSN returns POSTGRES_DSN or skips the test.
func postgresDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres live tests")
	}

	return dsn
}

// stubDB is a minimal coredb.DB backed by pgx, used by the live test to run
// Record inside a real transaction.
type stubDB struct {
	conn *pgx.Conn
}

func (d stubDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := d.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return stubRows{rows: rows}, nil
}

func (d stubDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	ct, err := d.conn.Exec(ctx, query, args...)

	return ct.RowsAffected(), err
}

func (d stubDB) Ping(ctx context.Context) error { return d.conn.Ping(ctx) }

func (d stubDB) Close(ctx context.Context) error { return d.conn.Close(ctx) }

func (d stubDB) Dialect() string { return "postgres" }

// BeginTx starts a pgx transaction.
func (d stubDB) BeginTx(ctx context.Context, _ *db.TxOptions) (db.Tx, error) {
	tx, err := d.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	return stubTx{tx: tx}, nil
}

// stubTx is a minimal coredb.Tx backed by pgx.
type stubTx struct {
	tx pgx.Tx
}

func (t stubTx) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return stubRows{rows: rows}, nil
}

func (t stubTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	ct, err := t.tx.Exec(ctx, query, args...)

	return ct.RowsAffected(), err
}

func (t stubTx) Close(context.Context) error { return nil }

func (t stubTx) Ping(context.Context) error { return nil }

func (t stubTx) Dialect() string { return "postgres" }

func (t stubTx) Commit(ctx context.Context) error { return t.tx.Commit(ctx) }

func (t stubTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

func (t stubTx) Savepoint(ctx context.Context, name string) error {
	_, err := t.tx.Exec(ctx, "SAVEPOINT "+name)

	return err
}

func (t stubTx) RollbackTo(ctx context.Context, name string) error {
	_, err := t.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name)

	return err
}

// stubRows is a minimal coredb.Rows backed by pgx.
type stubRows struct {
	rows pgx.Rows
}

func (r stubRows) Next() bool { return r.rows.Next() }

func (r stubRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }

func (r stubRows) Close() error {
	r.rows.Close()

	return nil
}

func (r stubRows) Columns() ([]string, error) {
	fields := r.rows.FieldDescriptions()
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}

	return names, nil
}

func (r stubRows) Err() error { return r.rows.Err() }

// openStubDB opens a pgx connection wrapped as a coredb.DB.
func openStubDB(t *testing.T, dsn string) stubDB {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("pgx.Connect() error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	return stubDB{conn: conn}
}

// requireWalLevelLogical skips the test unless the server has wal_level=logical.
func requireWalLevelLogical(t *testing.T, dsn string) {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Skipf("cannot connect to check wal_level: %v", err)
	}

	defer func() { _ = conn.Close(context.Background()) }()

	var level string
	if err := conn.QueryRow(t.Context(), "SHOW wal_level").Scan(&level); err != nil {
		t.Skipf("cannot read wal_level: %v", err)
	}

	if level != "logical" {
		t.Skipf("wal_level = %q, want logical", level)
	}
}

// skipOnPermissionError skips the test when err is a postgres permission error.
func skipOnPermissionError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		return
	}

	msg := err.Error()
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "must be superuser") ||
		strings.Contains(msg, "replication slots can only be used by superusers") {
		t.Skipf("managed postgres permission error: %v", err)
	}
}
