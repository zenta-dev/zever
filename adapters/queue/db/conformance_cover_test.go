package db_test

import (
	"os"
	"path/filepath"
	"testing"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	queuedb "github.com/zenta-dev/zever/adapters/queue/db"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/queue/queuetest"
)

// TestDBConformance proves the DB-backed queue honors the queue.Queue
// contract via the shared conformance kit. The sqlite leg runs on a fresh
// file-backed database per subtest (file paths isolate cases: ":memory:"
// sqlite uses shared cache, so distinct files keep them independent). The
// postgres leg runs only when POSTGRES_DSN names a live server.
func TestDBConformance(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		queuetest.Conformance(t, func(t *testing.T) queue.Queue {
			t.Helper()

			q, err := queuedb.New(queuedb.Options{
				Options:     coredb.Options{Path: filepath.Join(t.TempDir(), "queue.db")},
				PollTimeout: queuetest.DefaultPollTimeout,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			t.Cleanup(func() { _ = q.Close() })

			return q
		})
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("POSTGRES_DSN")
		if dsn == "" {
			t.Skip("POSTGRES_DSN not set; skipping postgres conformance against a live server")
		}

		// Kit subtests intentionally leave rows behind (LengthIsEmpty keeps
		// one ready message) on distinct topics, so start from a clean
		// table: otherwise reruns see the previous run's leftovers.
		admin, err := dbpostgres.New(coredb.Options{DSN: dsn})
		if err != nil {
			t.Fatalf("postgres New() error = %v", err)
		}

		ctx := t.Context()

		if _, err := admin.Exec(ctx, `DROP TABLE IF EXISTS "queue_messages"`); err != nil {
			t.Fatalf("drop table error = %v", err)
		}

		t.Cleanup(func() { _ = admin.Close(ctx) })

		queuetest.Conformance(t, func(t *testing.T) queue.Queue {
			t.Helper()

			q, err := queuedb.New(queuedb.Options{
				Options:     coredb.Options{DSN: dsn},
				PollTimeout: queuetest.DefaultPollTimeout,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			t.Cleanup(func() { _ = q.Close() })

			return q
		})
	})
}
