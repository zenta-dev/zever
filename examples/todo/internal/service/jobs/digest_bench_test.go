package jobs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/examples/todo/internal/service/jobs"
)

// benchDB opens a fresh sqlite file, applies the todo schema, and seeds a
// user with a mix of overdue, fresh, and done notes.
func benchDB(b *testing.B) (db.DB, context.Context) {
	b.Helper()

	dbsqlite.Register()

	ctx := b.Context()
	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(b.TempDir(), "bench.db")})
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		b.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(ctx, stmt); err != nil {
			b.Fatalf("migrate: %v", err)
		}
	}

	now := time.Now().UTC()
	if _, err := conn.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		"u-1", "u@example.com", "hash", now.Format(time.RFC3339Nano)); err != nil {
		b.Fatalf("seed user: %v", err)
	}
	old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	recent := now.Format(time.RFC3339Nano)
	for i, n := range []struct {
		id   string
		done int
		at   string
	}{
		{"n-1", 0, old},
		{"n-2", 0, old},
		{"n-3", 0, recent},
		{"n-4", 1, old},
	} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO notes (id, user_id, title, body, done, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			n.id, "u-1", "title", "body", n.done, n.at); err != nil {
			b.Fatalf("seed note %d: %v", i, err)
		}
	}

	return conn, ctx
}

// BenchmarkCountOverdue measures the overdue-note count query.
func BenchmarkCountOverdue(b *testing.B) {
	conn, ctx := benchDB(b)
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := jobs.CountOverdue(ctx, conn, now)
		if err != nil {
			b.Fatalf("CountOverdue: %v", err)
		}
		if n != 2 {
			b.Fatalf("count = %d, want 2", n)
		}
	}
}
