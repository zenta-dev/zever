package api_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	genapp "github.com/zenta-dev/zever/examples/bookings/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// benchConn opens a fresh sqlite file, applies the bookings schema, and seeds
// one host with several spaces.
func benchConn(b *testing.B) (db.DB, context.Context) {
	b.Helper()

	dbsqlite.Register()

	ctx := b.Context()
	conn, err := db.Open(db.SQLite, db.Options{Path: filepath.Join(b.TempDir(), "bench.db")})
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close(b.Context()) })

	raw, err := os.ReadFile(filepath.Join("testdata", "schema.sql"))
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
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		"host-1", "host@example.com", "hash", stamp); err != nil {
		b.Fatalf("seed user: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := conn.Exec(ctx,
			`INSERT INTO spaces (id, host_id, title, description, lat, lng, price_cents, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"s-"+string(rune('a'+i)), "host-1", "Loft", "sunny", 41.8781, -87.6298, int64(15000), stamp); err != nil {
			b.Fatalf("seed space: %v", err)
		}
	}

	return conn, ctx
}

// BenchmarkListSpaces measures the typed space listing behind GET /api/spaces.
func BenchmarkListSpaces(b *testing.B) {
	conn, ctx := benchConn(b)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := orm.From(genapp.Spaces).All(ctx, conn)
		if err != nil {
			b.Fatalf("All: %v", err)
		}
		if len(rows) != 20 {
			b.Fatalf("rows = %d, want 20", len(rows))
		}
	}
}
