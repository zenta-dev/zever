package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/seed"

	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/queue/memory"
)

// TestRunIsIdempotent runs the seeder twice and checks counts stay stable.
func TestRunIsIdempotent(t *testing.T) {
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}

	for i := range 2 {
		if err := seed.Run(ctx, database); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	count := func(query string, args ...any) int {
		t.Helper()
		rows, err := database.Query(ctx, query, args...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer func() { _ = rows.Close() }()
		if !rows.Next() {
			t.Fatalf("no rows")
		}
		var n int64
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		return int(n)
	}

	if n := count(`SELECT COUNT(*) FROM users WHERE email IN (?, ?)`, seed.HostEmail, seed.GuestEmail); n != 2 {
		t.Fatalf("users = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM spaces WHERE id IN (?, ?)`, seed.SpaceOneID, seed.SpaceTwoID); n != 2 {
		t.Fatalf("spaces = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM bookings WHERE id = ?`, seed.BookingID); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}
