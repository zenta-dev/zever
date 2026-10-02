package container

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/search"
)

// stubPool is a fake db.DB recording closes. It is safe for concurrent use.
type stubPool struct {
	mu     sync.Mutex
	closes int
}

func (s *stubPool) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nil, errors.New("stubpool: no rows")
}

func (s *stubPool) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 0, nil
}

func (s *stubPool) Ping(_ context.Context) error { return nil }

func (s *stubPool) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes++

	return nil
}

func (s *stubPool) Dialect() string { return "sqlite" }

// warnSink collects warn lines for assertions.
type warnSink struct {
	mu   sync.Mutex
	msgs []string
}

func (w *warnSink) warn(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.msgs = append(w.msgs, msg)
}

func (w *warnSink) all() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return append([]string(nil), w.msgs...)
}

// stubRegistry builds a registry whose opens run open and whose warns land
// in sink. It returns the registry and a counter of open calls.
func stubRegistry(open func(adapter db.Adapter, opts db.Options) (db.DB, error), sink *warnSink) (*poolRegistry, *atomic.Int32) {
	var calls atomic.Int32
	r := newPoolRegistry()
	r.warn = sink.warn
	prev := open
	r.open = func(adapter db.Adapter, opts db.Options) (db.DB, error) {
		calls.Add(1)

		return prev(adapter, opts)
	}

	return r, &calls
}

func mustBorrow(t *testing.T, r *poolRegistry, service string, opts db.Options, key, backend string) db.DB {
	t.Helper()

	conn, err := r.borrow(service, db.SQLite, opts, key, backend)
	if err != nil {
		t.Fatalf("borrow %s: %v", service, err)
	}

	return conn
}

func TestPoolKeys(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		wantKey     string
		wantBackend string
		wantOK      bool
	}{
		{name: "postgres exact", dsn: "postgres://app@db:5432/app", wantKey: "postgres:postgres://app@db:5432/app", wantBackend: poolPostgres, wantOK: true},
		{name: "postgres query differs", dsn: "postgres://app@db:5432/app?sslmode=require", wantKey: "postgres:postgres://app@db:5432/app?sslmode=require", wantBackend: poolPostgres, wantOK: true},
		{name: "postgres trims space", dsn: "  postgres://app@db:5432/app  ", wantKey: "postgres:postgres://app@db:5432/app", wantBackend: poolPostgres, wantOK: true},
		{name: "sqlite file", dsn: "data/app.db", wantKey: "sqlite:data/app.db", wantBackend: poolSQLite, wantOK: true},
		{name: "sqlite cleans path", dsn: "data/../data/app.db", wantKey: "sqlite:data/app.db", wantBackend: poolSQLite, wantOK: true},
		{name: "empty never shares", dsn: "", wantOK: false},
		{name: "blank never shares", dsn: "   ", wantOK: false},
		{name: "memory never shares", dsn: ":memory:", wantOK: false},
		{name: "shared memory never shares", dsn: "file::memory:?cache=shared", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, backend, ok := coreDSNPoolKey(tc.dsn)
			if ok != tc.wantOK || key != tc.wantKey || backend != tc.wantBackend {
				t.Errorf("coreDSNPoolKey(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.dsn, key, backend, ok, tc.wantKey, tc.wantBackend, tc.wantOK)
			}
		})
	}

	if key, ok := postgresPoolKey("file.db"); ok || key != "" {
		t.Errorf("postgresPoolKey(file.db) = (%q, %v), want no key", key, ok)
	}

	if key, ok := sqlitePoolKey(":memory:"); ok || key != "" {
		t.Errorf("sqlitePoolKey(:memory:) = (%q, %v), want no key", key, ok)
	}

	if key, ok := sqlitePoolKey(""); ok || key != "" {
		t.Errorf("sqlitePoolKey(empty) = (%q, %v), want no key", key, ok)
	}
}

func TestPostgresPreviewHidesSecrets(t *testing.T) {
	got := postgresPreview("postgres://app:s3cret@db:5432/app?sslmode=require")
	if got != "host=db dbname=app" {
		t.Errorf("postgresPreview = %q, want host/dbname only", got)
	}

	if strings.Contains(got, "s3cret") || strings.Contains(got, "sslmode") {
		t.Errorf("postgresPreview leaks secrets: %q", got)
	}

	if got := postgresPreview("not a url with spaces"); got != "host=unknown dbname=unknown" {
		t.Errorf("postgresPreview(bad) = %q, want unknowns", got)
	}
}

func TestPoolRegistry_BorrowSharesOnePool(t *testing.T) {
	sink := &warnSink{}
	r, calls := stubRegistry(func(_ db.Adapter, _ db.Options) (db.DB, error) {
		return &stubPool{}, nil
	}, sink)

	a := mustBorrow(t, r, "db", db.Options{Path: "f.db"}, "sqlite:f.db", poolSQLite)
	b := mustBorrow(t, r, "queue", db.Options{Path: "f.db"}, "sqlite:f.db", poolSQLite)

	if a != b {
		t.Fatal("same key must return the same pool")
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("open calls = %d, want 1", got)
	}

	snaps := r.snapshot()
	if len(snaps) != 1 {
		t.Fatalf("snapshot entries = %d, want 1", len(snaps))
	}

	if snaps[0].name != "pool shared by db,queue" {
		t.Errorf("snapshot name = %q, want service list", snaps[0].name)
	}

	if strings.Contains(snaps[0].name, "f.db") {
		t.Errorf("snapshot name leaks path: %q", snaps[0].name)
	}
}

func TestPoolRegistry_CapMismatchWarnsWithoutDSN(t *testing.T) {
	//nolint:gosec // fixture DSN with a fake password, never dialed
	const dsn = "postgres://app:s3cret@db:5432/app"
	sink := &warnSink{}
	r, _ := stubRegistry(func(_ db.Adapter, _ db.Options) (db.DB, error) {
		return &stubPool{}, nil
	}, sink)

	mustBorrow(t, r, "db", db.Options{DSN: dsn, MaxConns: 25}, "postgres:"+dsn, poolPostgres)
	mustBorrow(t, r, "queue", db.Options{DSN: dsn, MaxConns: 10}, "postgres:"+dsn, poolPostgres)
	// Unset caps never warn.
	mustBorrow(t, r, "search", db.Options{DSN: dsn}, "postgres:"+dsn, poolPostgres)

	var mismatch []string
	for _, m := range sink.all() {
		if strings.Contains(m, "ignored") {
			mismatch = append(mismatch, m)
		}
	}

	if len(mismatch) != 1 {
		t.Fatalf("mismatch warnings = %d (%v), want 1", len(mismatch), sink.all())
	}

	got := mismatch[0]
	if !strings.Contains(got, "queue") || !strings.Contains(got, "max_conns=10") {
		t.Errorf("warning names service and cap: %q", got)
	}

	if strings.Contains(got, "s3cret") || strings.Contains(got, dsn) {
		t.Errorf("warning leaks DSN: %q", got)
	}
}

func TestPoolRegistry_BuildFailureEvictsAndRetries(t *testing.T) {
	sink := &warnSink{}
	var calls atomic.Int32
	r := newPoolRegistry()
	r.warn = sink.warn
	r.open = func(_ db.Adapter, _ db.Options) (db.DB, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("boom")
		}

		return &stubPool{}, nil
	}

	_, err := r.borrow("queue", db.SQLite, db.Options{Path: "f.db"}, "sqlite:f.db", poolSQLite)
	if err == nil || !strings.Contains(err.Error(), "(shared pool)") || !strings.Contains(err.Error(), "queue") {
		t.Fatalf("borrow error = %v, want shared-pool marker naming queue", err)
	}

	if got := len(r.snapshot()); got != 0 {
		t.Fatalf("snapshot after failure = %d, want 0 (key evicted)", got)
	}

	if _, err := r.borrow("queue", db.SQLite, db.Options{Path: "f.db"}, "sqlite:f.db", poolSQLite); err != nil {
		t.Fatalf("retry borrow: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("open calls = %d, want 2 (failure not cached)", got)
	}
}

// stubCache is a Close-recording cache.Cache for close-order tests.
type stubCache struct {
	order *[]string
	mu    *sync.Mutex
}

func (s *stubCache) Get(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("stub") }

func (s *stubCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return errors.New("stub")
}

func (s *stubCache) SetIfAbsent(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return false, errors.New("stub")
}

func (s *stubCache) Delete(_ context.Context, _ string) error { return errors.New("stub") }

func (s *stubCache) Increment(_ context.Context, _ string) error { return errors.New("stub") }

func (s *stubCache) Decrement(_ context.Context, _ string) error { return errors.New("stub") }

func (s *stubCache) Exists(_ context.Context, _ string) (bool, error) {
	return false, errors.New("stub")
}

func (s *stubCache) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.order = append(*s.order, "cache")

	return nil
}

// recordingPool is a db.DB appending "pool" on Close.
type recordingPool struct {
	stubPool
	order *[]string
	mu    *sync.Mutex
}

func (s *recordingPool) Close(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.order = append(*s.order, "pool")

	return nil
}

func TestPoolRegistry_CloseOrdering(t *testing.T) {
	c := New(config.Default())
	c.pools.warn = func(string) {}

	var order []string
	var mu sync.Mutex
	c.cache.val = &stubCache{order: &order, mu: &mu}
	c.cache.done = true
	c.cache.ready.Store(true)

	pool := &recordingPool{order: &order, mu: &mu}
	c.pools.open = func(_ db.Adapter, _ db.Options) (db.DB, error) { return pool, nil }

	conn, err := c.pools.borrow("db", db.SQLite, db.Options{Path: "f.db"}, "sqlite:f.db", poolSQLite)
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	c.db.val = conn
	c.db.done = true
	c.db.ready.Store(true)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(order) != 2 || order[0] != "cache" || order[1] != "pool" {
		t.Fatalf("close order = %v, want [cache pool] exactly once", order)
	}
}

func TestContainer_DedicatedPoolIsolation(t *testing.T) {
	dbsqlite.Register()
	_ = queue.Register(queue.DB, func(_ queue.Options) (queue.Queue, error) {
		return &fakeQueue{}, nil
	})

	file := filepath.Join(t.TempDir(), "iso.db")
	cfg := config.Default()
	cfg.DB.Adapter = "sqlite"
	cfg.DB.Options = db.Options{Path: file}
	cfg.Queue.Adapter = "db"
	cfg.Queue.Options = queue.Options{DBOptions: queue.DBOptions{DSN: file, DedicatedPool: true}}

	c := New(cfg)
	c.pools.warn = func(string) {}

	if _, err := c.DB(); err != nil {
		t.Fatalf("DB: %v", err)
	}

	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}

	snaps := c.pools.snapshot()
	if len(snaps) != 1 {
		t.Fatalf("registry entries = %d, want 1 (dedicated queue isolated)", len(snaps))
	}

	if snaps[0].name != "pool shared by db" {
		t.Errorf("registry entry = %q, want db only", snaps[0].name)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// stubSearch is a no-op search.Search proving the borrower path without
// importing a search adapter.
type stubSearch struct{}

func (s *stubSearch) Index(_ context.Context, _ search.Document) error { return nil }

func (s *stubSearch) IndexBatch(_ context.Context, _ []search.Document) error { return nil }

func (s *stubSearch) Delete(_ context.Context, _ string) error { return nil }

func (s *stubSearch) Search(_ context.Context, _ string, _ search.QueryOptions) (search.Result, error) {
	return search.Result{}, nil
}

func (s *stubSearch) Close() error { return nil }

func TestContainer_SharedPool_SQLiteFile(t *testing.T) {
	// The shared path builds pools via c.pools.open and borrowers via the
	// batteries' OpenShared registries, so this test registers stub shared
	// constructors: no adapter import, so container tests never drag
	// adapter modules (and their SDKs) into downstream tidy graphs. No
	// other test file registers shared factories, and factory-registry
	// fakes for other adapters never shadow the separate shared
	// registries, so this stays deterministic regardless of which fake
	// adapters other test files registered globally (first registration
	// wins there). Real OpenFromDB behavior stays covered per-adapter.
	dbsqlite.Register()
	_ = cache.RegisterShared(cache.DB, func(_ db.DB, _ cache.Options) (cache.Cache, error) {
		return &fakeCache{}, nil
	})
	_ = queue.RegisterShared(queue.DB, func(_ db.DB, _ queue.Options) (queue.Queue, error) {
		return &fakeQueue{}, nil
	})
	_ = search.RegisterShared(search.DB, func(_ db.DB, _ search.Options) (search.Search, error) {
		return &stubSearch{}, nil
	})

	file := filepath.Join(t.TempDir(), "shared.db")
	cfg := config.Default()
	cfg.DB.Adapter = "sqlite"
	cfg.DB.Options = db.Options{Path: file}
	cfg.Queue.Adapter = "db"
	cfg.Queue.Options = queue.Options{DBOptions: queue.DBOptions{DSN: file}}
	cfg.Search.Adapter = string(search.DB)
	cfg.Search.Options = search.Options{DSN: file}
	cfg.Cache.Adapter = "db"
	cfg.Cache.Options = cache.Options{DSN: file}

	c := New(cfg)
	var warns warnSink
	c.pools.warn = warns.warn
	var opens atomic.Int32
	c.pools.open = func(_ db.Adapter, opts db.Options) (db.DB, error) {
		opens.Add(1)

		return dbsqlite.New(opts)
	}

	ctx := t.Context()
	must := func(op string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}

	d, err := c.DB()
	must("DB", err)

	if d == nil {
		t.Fatal("DB is nil")
	}

	if _, err := c.Queue(); err != nil {
		t.Fatalf("Queue: %v", err)
	}

	if _, err := c.Search(); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if _, err := c.Cache(); err != nil {
		t.Fatalf("Cache: %v", err)
	}

	if got := opens.Load(); got != 1 {
		t.Errorf("pool opens = %d, want 1 shared pool", got)
	}

	snaps := c.pools.snapshot()
	if len(snaps) != 1 {
		t.Fatalf("registry entries = %d, want 1 shared pool", len(snaps))
	}

	if snaps[0].name != "pool shared by db,queue,search,cache" {
		t.Errorf("registry entry = %q, want all four borrowers", snaps[0].name)
	}

	for _, m := range warns.all() {
		if strings.Contains(m, file) {
			t.Errorf("warn leaks path: %q", m)
		}
	}

	// The shared pool is a real sqlite pool: borrowers build over it.
	if _, err := d.Exec(ctx, "CREATE TABLE IF NOT EXISTS probe (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec over shared pool: %v", err)
	}

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping shared pool: %v", err)
	}

	closeCtx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	must("Close", c.Close(closeCtx))
}
