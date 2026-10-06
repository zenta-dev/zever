package seed_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/seed"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

// errSeedCover is the generic database failure the seed cover DB reports.
var errSeedCover = errors.New("cover: database failure")

// seedCoverDB delegates to a real database but fails queries/execs whose SQL
// contains the configured substring, pinning lookup/insert error branches.
type seedCoverDB struct {
	db.DB
	failQuery string
	failExec  string
}

func (c seedCoverDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	if c.failQuery != "" && strings.Contains(strings.ToLower(query), strings.ToLower(c.failQuery)) {
		return nil, errSeedCover
	}
	return c.DB.Query(ctx, query, args...)
}

func (c seedCoverDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if c.failExec != "" && strings.Contains(strings.ToLower(query), c.failExec) {
		return 0, errSeedCover
	}
	return c.DB.Exec(ctx, query, args...)
}

// newSeedCoverDB opens a migrated sqlite file for seed error-path tests.
func newSeedCoverDB(t *testing.T) db.DB {
	t.Helper()

	dbsqlite.Register()
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "cover.db")
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
	return database
}

// TestSeedRunLookupErrors pins the lookup-failed branches of every ensure step.
func TestSeedRunLookupErrors(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{"user lookup", "users", "seed: lookup user"},
		{"space lookup", "spaces", "seed: lookup space"},
		{"booking lookup", "bookings", "seed: lookup booking"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			database := newSeedCoverDB(t)
			err := seed.Run(t.Context(), seedCoverDB{DB: database, failQuery: tc.table})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestSeedRunInsertErrors pins the insert-failed branches of every ensure step.
func TestSeedRunInsertErrors(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{"user insert", "users", "seed: insert user"},
		{"space insert", "spaces", "seed: insert space"},
		{"booking insert", "bookings", "seed: insert booking"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			database := newSeedCoverDB(t)
			err := seed.Run(t.Context(), seedCoverDB{DB: database, failExec: tc.table})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run = %v, want %q", err, tc.want)
			}
		})
	}
}
