package jobs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/todo/internal/service/jobs"
	"github.com/zenta-dev/zever/examples/todo/internal/testsetup"
)

func newTestDB(t *testing.T) (*container.Container, context.Context) {
	t.Helper()

	testsetup.RegisterDefaults()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	c := container.New(cfg)

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	logger, err := c.Log()
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	_ = logger

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

	t.Cleanup(func() { _ = c.Close(t.Context()) })
	return c, ctx
}

func insertNote(t *testing.T, c *container.Container, userID, createdAt string, done int) {
	t.Helper()
	ctx := t.Context()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	if _, execErr := database.Exec(ctx,
		`INSERT INTO notes (id, user_id, title, body, done, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), userID, "title", "body", done, createdAt,
	); execErr != nil {
		t.Fatalf("insert note: %v", execErr)
	}
}

// TestRunDigestCountsOnlyOldUndoneNotes seeds two overdue notes alongside
// recent and done decoys, then invokes the handler directly.
func TestRunDigestCountsOnlyOldUndoneNotes(t *testing.T) {
	c, ctx := newTestDB(t)
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	logger, logErr := c.Log()
	if logErr != nil {
		t.Fatalf("Log: %v", logErr)
	}

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	recent := now.Format(time.RFC3339Nano)

	userID := uuid.NewString()
	if _, execErr := database.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, "demo@example.com", "hash", recent,
	); execErr != nil {
		t.Fatalf("insert user: %v", execErr)
	}

	insertNote(t, c, userID, old, 0)
	insertNote(t, c, userID, old, 0)
	insertNote(t, c, userID, recent, 0)
	insertNote(t, c, userID, old, 1)

	n, countErr := jobs.CountOverdue(ctx, database, now)
	if countErr != nil {
		t.Fatalf("CountOverdue: %v", countErr)
	}
	if n != 2 {
		t.Fatalf("CountOverdue = %d, want 2", n)
	}

	n, runErr := jobs.RunDigest(ctx, database, logger)
	if runErr != nil {
		t.Fatalf("RunDigest: %v", runErr)
	}
	if n != 2 {
		t.Fatalf("RunDigest = %d, want 2", n)
	}

	handle := jobs.Handler(database, logger)
	if handleErr := handle(ctx, jobs.SendOverdueDigestArgs{}); handleErr != nil {
		t.Fatalf("Handler: %v", handleErr)
	}
}
