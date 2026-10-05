package seed_test

import (
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

// TestEnsureJoinTableCreatesAndIsIdempotent proves the hand-owned
// product_tags join table is created on demand and that a second call is a
// no-op (CREATE TABLE IF NOT EXISTS semantics).
func TestEnsureJoinTableCreatesAndIsIdempotent(t *testing.T) {
	dbsqlite.Register()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "join.db")
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	ctx := t.Context()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	if seedErr := seed.EnsureJoinTable(ctx, database); seedErr != nil {
		t.Fatalf("EnsureJoinTable: %v", seedErr)
	}

	if seedErr := seed.EnsureJoinTable(ctx, database); seedErr != nil {
		t.Fatalf("EnsureJoinTable second call: %v", seedErr)
	}

	rows, err := database.Query(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'product_tags'`)
	if err != nil {
		t.Fatalf("join lookup: %v", err)
	}

	var found int
	if rows.Next() {
		if err := rows.Scan(&found); err != nil {
			_ = rows.Close()
			t.Fatalf("scan: %v", err)
		}
	}
	_ = rows.Close()

	if found != 1 {
		t.Fatalf("product_tags table count = %d, want 1", found)
	}
}
