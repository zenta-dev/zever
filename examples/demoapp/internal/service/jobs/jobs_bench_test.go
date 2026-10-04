package jobs_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/jobs"
)

// benchDeps opens a fresh sqlite file, applies the demoapp schema, seeds a
// handful of rows, and returns job dependencies with a discarded logger.
func benchDeps(b *testing.B) jobs.Deps {
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

	const stamp = "2026-01-02T03:04:05Z"
	if _, err := conn.Exec(ctx,
		`INSERT INTO categories (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		"cat-1", "Gadgets", "gadgets", stamp); err != nil {
		b.Fatalf("seed category: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := conn.Exec(ctx,
			`INSERT INTO products (id, category_id, name, description, price_cents, stock, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"p-"+string(rune('a'+i)), "cat-1", "Widget", "desc", 2500, 10, stamp); err != nil {
			b.Fatalf("seed product: %v", err)
		}
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"u-1", "u@example.com", "U", "member", "hash", stamp); err != nil {
		b.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO orders (id, user_id, total_cents, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		"o-1", "u-1", 2500, "pending", stamp); err != nil {
		b.Fatalf("seed order: %v", err)
	}

	return jobs.Deps{DB: conn, Logger: logslog.NewWithWriter(log.Options{}, io.Discard)}
}

// BenchmarkRunReindex measures the product-list scan that stands in for
// search-index maintenance.
func BenchmarkRunReindex(b *testing.B) {
	deps := benchDeps(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := jobs.RunReindex(ctx, deps); err != nil {
			b.Fatalf("RunReindex: %v", err)
		}
	}
}

// BenchmarkRunDailyReport measures the pending-order scan.
func BenchmarkRunDailyReport(b *testing.B) {
	deps := benchDeps(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := jobs.RunDailyReport(ctx, deps); err != nil {
			b.Fatalf("RunDailyReport: %v", err)
		}
	}
}
