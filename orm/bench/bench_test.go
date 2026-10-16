package bench

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// benchWidget mirrors the shape schema codegen generates: a plain struct,
// a package-level Table, one Column per field, and a Scan method.
type benchWidget struct {
	ID   string
	Name string
	Qty  int64
}

func (w *benchWidget) Scan(row orm.Row) error { return row.Scan(&w.ID, &w.Name, &w.Qty) }

var (
	benchWidgets = orm.NewTable[benchWidget]("widgets", []string{"id", "name", "qty"})
	benchID      = orm.NewColumn[benchWidget, string]("widgets", "id")
	benchName    = orm.NewColumn[benchWidget, string]("widgets", "name")
	benchQty     = orm.NewColumn[benchWidget, int64]("widgets", "qty")
)

// openBenchDB opens a fresh in-memory sqlite database seeded with n
// widgets (ids w000000..). Each benchmark gets its own database, so runs
// never share mutable state.
func openBenchDB(b *testing.B, n int) (context.Context, db.DB) {
	b.Helper()

	ctx := b.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		b.Fatalf("open: %v", err)
	}

	b.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, qty integer)`); err != nil {
		b.Fatalf("create table: %v", err)
	}

	const batch = 500

	args := make([]any, 0, batch*3)

	var s strings.Builder

	for i := 0; i < n; i += batch {
		s.Reset()
		args = args[:0]
		s.WriteString("INSERT INTO widgets (id, name, qty) VALUES ")

		for j := i; j < min(i+batch, n); j++ {
			if j > i {
				s.WriteString(", ")
			}

			s.WriteString("(?, ?, ?)")
			args = append(args, fmt.Sprintf("w%06d", j), fmt.Sprintf("widget %d", j), int64(j))
		}

		if _, err := conn.Exec(ctx, s.String(), args...); err != nil {
			b.Fatalf("seed: %v", err)
		}
	}

	return ctx, conn
}

// drain runs a raw SELECT and scans every row, so the raw baseline pays
// the same materialization cost as the ORM path.
func drain(ctx context.Context, b *testing.B, conn db.DB, query string, args ...any) {
	b.Helper()

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		b.Fatalf("query: %v", err)
	}

	var id, name string

	var qty int64

	for rows.Next() {
		if err := rows.Scan(&id, &name, &qty); err != nil {
			_ = rows.Close()
			b.Fatalf("scan: %v", err)
		}
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		b.Fatalf("rows: %v", err)
	}

	if err := rows.Close(); err != nil {
		b.Fatalf("close: %v", err)
	}
}

func BenchmarkZeverPointLookup(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, _, err := orm.From(benchWidgets).Where(benchID.Eq("w000500")).First(ctx, conn); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawPointLookup(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		drain(ctx, b, conn, `SELECT "id", "name", "qty" FROM "widgets" WHERE "id" = ? LIMIT 1`, "w000500")
	}
}

func BenchmarkZeverFilteredOrderLimit(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := orm.From(benchWidgets).Where(benchQty.Gt(100)).OrderBy(benchQty.Desc()).Limit(20).All(ctx, conn); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawFilteredOrderLimit(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		drain(ctx, b, conn, `SELECT "id", "name", "qty" FROM "widgets" WHERE "qty" > ? ORDER BY "qty" DESC LIMIT 20`, int64(100))
	}
}

func BenchmarkZeverCount(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := orm.From(benchWidgets).Where(benchQty.Gt(100)).Count(ctx, conn); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawCount(b *testing.B) {
	ctx, conn := openBenchDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rows, err := conn.Query(ctx, `SELECT COUNT(*) FROM "widgets" WHERE "qty" > ?`, int64(100))
		if err != nil {
			b.Fatal(err)
		}

		var n int64

		if rows.Next() {
			if err := rows.Scan(&n); err != nil {
				_ = rows.Close()
				b.Fatal(err)
			}
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			b.Fatal(err)
		}

		if err := rows.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkZeverInsert(b *testing.B) {
	ctx, conn := openBenchDB(b, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		id := fmt.Sprintf("new-%d", i)
		if err := orm.InsertInto(benchWidgets).Values(orm.Set(benchID, id), orm.Set(benchName, id), orm.Set(benchQty, int64(i))).Exec(ctx, conn); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawInsert(b *testing.B) {
	ctx, conn := openBenchDB(b, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		id := fmt.Sprintf("new-%d", i)
		if _, err := conn.Exec(ctx, `INSERT INTO "widgets" ("id", "name", "qty") VALUES (?, ?, ?)`, id, id, int64(i)); err != nil {
			b.Fatal(err)
		}
	}
}
