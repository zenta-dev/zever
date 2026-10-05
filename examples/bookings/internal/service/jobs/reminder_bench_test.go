package jobs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/jobs"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

// benchBookingDB opens a migrated sqlite database seeded with one confirmed
// booking.
func benchBookingDB(b *testing.B) (db.DB, context.Context) {
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
		if _, execErr := conn.Exec(ctx, stmt); execErr != nil {
			b.Fatalf("migrate: %v", execErr)
		}
	}

	const stamp = "2026-01-02T03:04:05Z"
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{"host-1", "host@example.com", "hash", stamp}},
		{`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, []any{"guest-1", "guest@example.com", "hash", stamp}},
		{`INSERT INTO spaces (id, host_id, title, description, lat, lng, price_cents, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"space-1", "host-1", "Cabin", "Woods cabin", 1.0, 2.0, int64(10000), stamp}},
		{`INSERT INTO bookings (id, space_id, guest_id, start_date, end_date, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, []any{"booking-1", "space-1", "guest-1", "2030-06-01", "2030-06-03", "confirmed", stamp}},
	} {
		if _, execErr := conn.Exec(ctx, q.query, q.args...); execErr != nil {
			b.Fatalf("seed: %v", execErr)
		}
	}

	return conn, ctx
}

// BenchmarkDueBookings measures the confirmed-bookings scan and window
// filter.
func BenchmarkDueBookings(b *testing.B) {
	database, ctx := benchBookingDB(b)
	now := time.Date(2030, 5, 30, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := jobs.DueBookings(ctx, database, now); err != nil {
			b.Fatalf("DueBookings: %v", err)
		}
	}
}
