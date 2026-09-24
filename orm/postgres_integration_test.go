package orm

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/db"
	dbpostgres "github.com/zenta-dev/zever/db/postgres"
)

type pgWidget struct {
	ID   string
	Name string
	Qty  int64
}

func (w *pgWidget) Scan(row Row) error { return row.Scan(&w.ID, &w.Name, &w.Qty) }

// TestPostgresIntegration exercises the builder end to end against a live
// postgres: $n placeholders, predicate/order/limit select, count, and
// upsert with RETURNING. Set POSTGRES_DSN to run; skipped otherwise.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run postgres integration tests")
	}

	conn, err := dbpostgres.New(db.Options{DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	ctx := t.Context()
	t.Cleanup(func() { _ = conn.Close(ctx) })

	if d := conn.Dialect(); d != "postgres" {
		t.Fatalf("Dialect = %q, want postgres", d)
	}

	_, err = conn.Exec(ctx, `DROP TABLE IF EXISTS orm_pg_widgets`)
	if err != nil {
		t.Fatalf("drop: %v", err)
	}
	_, err = conn.Exec(ctx, `CREATE TABLE orm_pg_widgets (id text PRIMARY KEY, name text, qty bigint)`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(ctx, `DROP TABLE orm_pg_widgets`) })

	widgets := NewTable[pgWidget]("orm_pg_widgets", []string{"id", "name", "qty"})
	wid := NewColumn[pgWidget, string]("orm_pg_widgets", "id")
	wname := NewColumn[pgWidget, string]("orm_pg_widgets", "name")
	wqty := NewColumn[pgWidget, int64]("orm_pg_widgets", "qty")

	ins := InsertInto(widgets).
		Values(Set(wid, "w1"), Set(wname, "Alpha"), Set(wqty, int64(10))).
		Values(Set(wid, "w2"), Set(wname, "Beta"), Set(wqty, int64(3)))
	err = ins.Exec(ctx, conn)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := From(widgets).Where(wqty.Gt(int64(5))).OrderBy(wid.Asc()).Limit(10).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Alpha" {
		t.Fatalf("All = %+v, want Alpha only", got)
	}

	n, err := From(widgets).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}

	ret, err := InsertInto(widgets).
		Values(Set(wid, "w1"), Set(wname, "Alpha2"), Set(wqty, int64(11))).
		OnConflict(wid.Col()).DoUpdate(Set(wname, "Alpha2"), Set(wqty, int64(11))).
		Returning(wid.Col(), wname.Col(), wqty.Col()).
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("upsert returning: %v", err)
	}
	if len(ret) != 1 || ret[0].Name != "Alpha2" || ret[0].Qty != 11 {
		t.Fatalf("upsert = %+v, want updated Alpha2/11", ret)
	}
}
