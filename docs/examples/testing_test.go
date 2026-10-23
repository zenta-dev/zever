package examples_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/db/dbtest"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

// TestContainer_defaultCache mirrors the testing guide's container setup:
// wire the in-memory adapter via container.Provide, resolve from the
// container, and close in Cleanup. It doubles as the compile check for
// the guide's provider snippet.
func TestContainer_defaultCache(t *testing.T) {
	cfg := config.Default()

	c := container.New(cfg, container.Provide(cache.DefaultRef(), cachememory.New))
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	ch, err := c.Cache()
	if err != nil {
		t.Fatalf("Cache: %v", err)
	}
	ctx := t.Context()
	if err := ch.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := ch.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Get = %q, want %q", got, "v")
	}
}

// TestSQLite_perTestFile mirrors the guide's per-test sqlite pattern: one
// file under t.TempDir() per test, closed in Cleanup.
func TestSQLite_perTestFile(t *testing.T) {
	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	ctx := t.Context()
	if _, err := conn.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO users (name) VALUES (?)", "ada"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := conn.Query(ctx, "SELECT name FROM users")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	found := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "ada" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected row ada not found")
	}
}

// TestDB_sqliteConformance runs the db conformance kit against a fresh
// per-test sqlite file, proving the guide's kit snippet compiles and passes.
func TestDB_sqliteConformance(t *testing.T) {
	t.Parallel()

	dbtest.Conformance(t, func(t *testing.T) coredb.DB {
		conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "kit.db")})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close(t.Context()) })
		return conn
	})
}

// stubOutboxStore publishes synchronously on Record, mirroring the memory
// adapter's non-durable path, so the kit exercises its non-durable branch
// without adding an adapter dependency to this module.
type stubOutboxStore struct {
	pub outbox.Publisher
}

func (s *stubOutboxStore) Record(ctx context.Context, _ coredb.Tx, msg outbox.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	return s.pub.Publish(ctx, msg)
}

func (s *stubOutboxStore) Start(context.Context) error { return nil }
func (s *stubOutboxStore) Status() outbox.Status       { return outbox.Status{} }
func (s *stubOutboxStore) Close() error                { return nil }
func (s *stubOutboxStore) Name() string                { return "stub" }

// TestOutbox_conformance runs the outbox kit against the stub store, proving
// the guide's Harness snippet shape compiles and passes.
func TestOutbox_conformance(t *testing.T) {
	t.Parallel()

	outboxtest.Conformance(t, func(_ *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		return outboxtest.Harness{
			Store: &stubOutboxStore{pub: pub},
			InTx: func(ctx context.Context, fn func(context.Context, coredb.Tx) error) error {
				return fn(ctx, nil)
			},
		}
	})
}

// TestRouter_httptest mirrors the guide's httptest snippet shape against a
// plain stdlib mux: routes serve in-process with no port or network.
func TestRouter_httptest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("user " + req.PathValue("id")))
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/users/42", nil))
	if rec.Body.String() != "user 42" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "user 42")
	}
}
