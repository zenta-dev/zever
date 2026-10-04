package sqlite

import (
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// BenchmarkExec measures a single-row insert into an in-memory database.
func BenchmarkExec(b *testing.B) {
	d := newMemoryDB(b)
	ctx := b.Context()

	if _, err := d.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Exec(ctx, "INSERT INTO t (v) VALUES (?)", "bench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkQuery measures a single-row select from an in-memory database.
func BenchmarkQuery(b *testing.B) {
	d := newMemoryDB(b)
	ctx := b.Context()

	if _, err := d.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		b.Fatal(err)
	}

	if _, err := d.Exec(ctx, "INSERT INTO t (v) VALUES (?)", "bench"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rows, err := d.Query(ctx, "SELECT id, v FROM t WHERE id = ?", 1)
		if err != nil {
			b.Fatal(err)
		}

		for rows.Next() {
		}

		if err := rows.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPrepare measures the statement-cache hit path.
func BenchmarkPrepare(b *testing.B) {
	d := newMemoryDB(b)
	ctx := b.Context()

	preparer, ok := d.(db.Preparer)
	if !ok {
		b.Fatal("adapter does not implement db.Preparer")
	}

	if _, err := d.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		b.Fatal(err)
	}

	if _, err := preparer.Prepare(ctx, "SELECT v FROM t WHERE id = ?"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stmt, err := preparer.Prepare(ctx, "SELECT v FROM t WHERE id = ?")
		if err != nil {
			b.Fatal(err)
		}

		if err := stmt.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
