package seed_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"

	"github.com/zenta-dev/zever/examples/demoapp/internal/service/seed"
)

// errCover is the generic database failure coverDB reports.
var errCover = errors.New("cover: database failure")

// coverDB delegates to a real database but fails queries/execs whose SQL
// contains the configured substring, pinning lookup/insert error branches.
type coverDB struct {
	db.DB
	failQuery string
	failExec  string
}

func (c coverDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	if c.failQuery != "" && strings.Contains(strings.ToLower(query), c.failQuery) {
		return nil, errCover
	}
	return c.DB.Query(ctx, query, args...)
}

func (c coverDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if c.failExec != "" && strings.Contains(strings.ToLower(query), c.failExec) {
		return 0, errCover
	}
	return c.DB.Exec(ctx, query, args...)
}

// newCoverDB opens a migrated sqlite file for seed error-path tests.
func newCoverDB(t *testing.T) db.DB {
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
	migrate(t, database)
	return database
}

// TestRunLookupErrors pins the lookup-failed branches of every ensure step.
func TestRunLookupErrors(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{"user lookup", "users", "seed: lookup user"},
		{"category lookup", "categories", "seed: lookup category"},
		{"product lookup", "products", "seed: lookup product"},
		{"post lookup", "posts", "seed: lookup post"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			database := newCoverDB(t)
			err := seed.Run(t.Context(), coverDB{DB: database, failQuery: tc.table})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestRunInsertErrors pins the insert-failed branches of every ensure step.
func TestRunInsertErrors(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{"user insert", "users", "seed: insert user"},
		{"category insert", "categories", "seed: insert category"},
		{"product insert", "products", "seed: insert product"},
		{"post insert", "posts", "seed: insert post"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			database := newCoverDB(t)
			err := seed.Run(t.Context(), coverDB{DB: database, failExec: tc.table})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run = %v, want %q", err, tc.want)
			}
		})
	}
}
