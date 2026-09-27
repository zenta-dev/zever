package seed_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/todo/internal/service/seed"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

// TestRunIsIdempotent runs the seeder twice and checks row counts stay stable.
func TestRunIsIdempotent(t *testing.T) {
	dbsqlite.Register()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	ctx := t.Context()
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

	if n := count(`SELECT COUNT(*) FROM users WHERE email = ?`, seed.DemoEmail); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM notes WHERE id LIKE 'seed-note-%'`); n != 3 {
		t.Fatalf("notes = %d, want 3", n)
	}
}
