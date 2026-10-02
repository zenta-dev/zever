package container

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Pool backends behind a shared registry entry.
const (
	poolPostgres = "postgres"
	poolSQLite   = "sqlite"
)

// isMemorySQLite reports sqlite paths that must never share a pool.
// ":memory:" forces a single pooled connection because extra connections
// get isolated databases; sharing would silently merge logically separate
// stores. "file::memory:" is the same database under another name.
func isMemorySQLite(path string) bool {
	p := strings.TrimSpace(path)

	return p == ":memory:" || strings.HasPrefix(p, "file::memory:")
}

// postgresPoolKey maps a postgres DSN to its registry key. Matching is the
// exact string after trim with no URL canonicalization: the same URL with
// and without a query string are different pools.
func postgresPoolKey(dsn string) (string, bool) {
	t := strings.TrimSpace(dsn)
	if t == "" || !dbconn.IsPostgresDSN(t) {
		return "", false
	}

	return "postgres:" + t, true
}

// sqlitePoolKey maps a sqlite path to its registry key after filepath.Clean.
// Relative paths resolve against the process working directory: every
// battery must run with the same CWD for relative paths to share.
func sqlitePoolKey(path string) (string, bool) {
	t := strings.TrimSpace(path)
	if t == "" || isMemorySQLite(t) {
		return "", false
	}

	return "sqlite:" + filepath.Clean(t), true
}

// coreDSNPoolKey maps a single-string core DSN (postgres URL or sqlite path,
// empty meaning a private in-memory database) to its registry key and
// backend. Empty and :memory: DSNs never share.
func coreDSNPoolKey(dsn string) (key, backend string, ok bool) {
	t := strings.TrimSpace(dsn)
	if t == "" || isMemorySQLite(t) {
		return "", "", false
	}

	if dbconn.IsPostgresDSN(t) {
		return "postgres:" + t, poolPostgres, true
	}

	return "sqlite:" + filepath.Clean(t), poolSQLite, true
}

// postgresPreview renders the host/dbname-only display for a postgres DSN.
// Userinfo and query parameters never appear: the full DSN never leaves the
// process in logs or errors.
func postgresPreview(dsn string) string {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil || u.Hostname() == "" {
		return "host=unknown dbname=unknown"
	}

	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		name = "unknown"
	}

	return "host=" + u.Hostname() + " dbname=" + name
}

// sharedPoolOpts maps a core DSN onto pool options for a registry build,
// carrying the opt-out flag so adapter options stay faithful.
func sharedPoolOpts(dsn string, dedicated bool) db.Options {
	opts := dbconn.SplitDSN(dsn)
	opts.DedicatedPool = dedicated

	return opts
}

// poolEntry is one shared pool: the opened db.DB, the first opener's knobs,
// and the borrowing services for observability. ready closes when the entry
// leaves the in-flight state; building reports an in-flight open.
type poolEntry struct {
	backend  string
	conn     db.DB
	knobs    db.Options
	services []string
	building bool
	ready    chan struct{}
}

// poolSnapshot names one resolved pool for Close without ever including DSN
// or path content.
type poolSnapshot struct {
	name string
	conn db.DB
}

// poolRegistry maps shareable DSN/path keys to one shared db.DB pool per
// key. It is safe for concurrent use: first opener builds while concurrent
// borrowers wait on the entry channel (same shape as lazy.go); a build
// failure evicts the key so the next access retries.
type poolRegistry struct {
	mu    sync.Mutex
	pools map[string]*poolEntry
	// open builds a fresh pool. Production resolves via the db registry so
	// the container wires no adapters itself; tests inject a fake.
	open func(adapter db.Adapter, opts db.Options) (db.DB, error)
	// warn reports pool lifecycle notes (shared opens, cap mismatches,
	// dedicated opens). Default writes one line to stderr without DSN
	// content; tests inject a recorder.
	warn func(msg string)
}

// newPoolRegistry builds an empty registry with production defaults.
func newPoolRegistry() *poolRegistry {
	return &poolRegistry{
		pools: make(map[string]*poolEntry),
		open:  db.Open,
		warn:  func(msg string) { fmt.Fprintln(os.Stderr, msg) },
	}
}

// borrow returns the shared pool for key, opening it on first use with the
// first opener's knobs. Later borrowers reuse the pool: their pool knobs
// are ignored, and a nonzero MaxConns differing from the pool's warns
// without DSN content. Every successful borrow records the service.
func (r *poolRegistry) borrow(service string, adapter db.Adapter, opts db.Options, key, backend string) (db.DB, error) {
	for {
		r.mu.Lock()
		if r.pools == nil {
			r.pools = make(map[string]*poolEntry)
		}

		e, ok := r.pools[key]
		if !ok {
			e = &poolEntry{backend: backend, building: true, ready: make(chan struct{})}
			r.pools[key] = e
			r.mu.Unlock()

			conn, err := r.open(adapter, opts)
			r.mu.Lock()
			if err != nil {
				delete(r.pools, key)
				r.mu.Unlock()
				close(e.ready)

				return nil, fmt.Errorf("container: %s (shared pool): %w", service, err)
			}
			e.conn = conn
			e.knobs = opts
			e.services = append(e.services, service)
			e.building = false
			services := append([]string(nil), e.services...)
			knobs := e.knobs
			r.mu.Unlock()
			close(e.ready)
			r.logOpen(e.backend, key, service, services, knobs)

			return conn, nil
		}

		if e.building {
			ready := e.ready
			r.mu.Unlock()
			<-ready

			continue
		}

		if !containsService(e.services, service) {
			e.services = append(e.services, service)
		}
		conn, knobs := e.conn, e.knobs
		services := append([]string(nil), e.services...)
		r.mu.Unlock()
		if opts.MaxConns != 0 && opts.MaxConns != knobs.MaxConns {
			r.warn(fmt.Sprintf("container: %s (shared pool): max_conns=%d ignored, shared pool max_conns=%d",
				service, opts.MaxConns, knobs.MaxConns))
		}
		r.logOpen(e.backend, key, service, services, knobs)

		return conn, nil
	}
}

// logOpen emits one observability line per shared-pool open or join. The
// line carries only the parsed host/dbname (postgres) or the backend
// (sqlite): full DSNs and file paths never appear.
func (r *poolRegistry) logOpen(backend, key, _ string, services []string, knobs db.Options) {
	list := "[" + strings.Join(services, ",") + "]"
	if backend == poolPostgres {
		preview := postgresPreview(strings.TrimPrefix(key, "postgres:"))
		r.warn(fmt.Sprintf("container: pool shared postgres %s max_conns=%d services=%s",
			preview, knobs.MaxConns, list))

		return
	}

	r.warn(fmt.Sprintf("container: pool shared sqlite max_conns=%d services=%s",
		knobs.MaxConns, list))
}

// snapshot returns resolved (non-building) pools in deterministic key order
// for Close. In-flight builds are skipped: they were never resolved.
func (r *poolRegistry) snapshot() []poolSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]string, 0, len(r.pools))
	for k, e := range r.pools {
		if !e.building {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	out := make([]poolSnapshot, 0, len(keys))
	for _, k := range keys {
		e := r.pools[k]
		out = append(out, poolSnapshot{
			name: "pool shared by " + strings.Join(e.services, ","),
			conn: e.conn,
		})
	}

	return out
}

// containsService reports whether services already names service.
func containsService(services []string, service string) bool {
	for _, s := range services {
		if s == service {
			return true
		}
	}

	return false
}

// isDBAdapter reports whether adapter names one of the db-backed adapters
// for service.
func isDBAdapter(adapter string, names ...string) bool {
	for _, n := range names {
		if adapter == n {
			return true
		}
	}

	return false
}

// openShared borrows (or opens) the registry pool for an exact-DSN match
// and builds the service over it via fromDB with owns=false. It reports
// shared=false when the caller falls back to openService: a non-db-backed
// adapter, an explicit DedicatedPool opt-out, or a non-shareable DSN
// (empty or :memory:). A dedicated opt-out on a db-backed adapter logs one
// line naming the service only.
func openShared[T any](c *Container, service, adapter string, dbAdapters []string, key, backend string, shareable bool, dedicated bool, poolOpts db.Options, fromDB func(conn db.DB) (T, error)) (T, bool, error) {
	var zero T

	if !isDBAdapter(adapter, dbAdapters...) {
		return zero, false, nil
	}

	if dedicated {
		c.pools.warn(fmt.Sprintf("container: pool dedicated service=%s backend=%s", service, backend))

		return zero, false, nil
	}

	if !shareable {
		return zero, false, nil
	}

	var poolAdapter db.Adapter
	if backend == poolPostgres {
		poolAdapter = db.Postgres
	} else {
		poolAdapter = db.SQLite
	}

	conn, err := c.pools.borrow(service, poolAdapter, poolOpts, key, backend)
	if err != nil {
		return zero, true, err
	}

	//nolint:contextcheck // single ctx-free bridge by design: lazy accessors resolve with no caller ctx and adapter constructors take none (frozen public shapes), so there is no context to propagate — the same intentionally ctx-free boundary as the teardown callback in adapters/eventbus/redis/redis.go and the shared singleflight fetch in adapters/i18n/remote/remote.go. Pool opens use bounded internal timeouts instead of a caller ctx.
	v, err := fromDB(conn)
	if err != nil {
		return zero, true, fmt.Errorf("container: %s (shared pool): %w", service, err)
	}

	return v, true, nil
}

// openDB resolves the db service, registering shareable pools in the
// registry for borrowers. DedicatedPool is meaningless on db itself and is
// ignored. Non-shareable pools (:memory:) open directly and close with the
// registry band via the db lazy field.
func (c *Container) openDB() (db.DB, error) {
	adapterName := c.cfg.DB.Adapter
	opts := c.cfg.DB.Options

	switch db.Adapter(adapterName) {
	case db.Postgres:
		key, ok := postgresPoolKey(opts.DSN)
		if !ok {
			return openService("db", adapterName, db.ParseAdapter, db.Open, opts)
		}

		return c.pools.borrow("db", db.Postgres, opts, key, poolPostgres)
	case db.SQLite:
		path := strings.TrimSpace(opts.Path)
		if path == "" {
			path = "app.db"
		}
		key, ok := sqlitePoolKey(path)
		if !ok {
			return openService("db", adapterName, db.ParseAdapter, db.Open, opts)
		}

		return c.pools.borrow("db", db.SQLite, opts, key, poolSQLite)
	default:
		return openService("db", adapterName, db.ParseAdapter, db.Open, opts)
	}
}

// openSharedCache resolves the cache service over the shared pool on an
// exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedCache() (cache.Cache, bool, error) {
	o := c.cfg.Cache.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)
	poolOpts := sharedPoolOpts(o.DSN, o.DedicatedPool)

	return openShared(c, "cache", c.cfg.Cache.Adapter, []string{string(cache.DB)},
		key, backend, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (cache.Cache, error) {
			return cache.OpenShared(cache.Adapter(c.cfg.Cache.Adapter), conn, o)
		})
}

// openSharedQueue resolves the queue service over the shared pool on an
// exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedQueue() (queue.Queue, bool, error) {
	o := c.cfg.Queue.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)
	poolOpts := sharedPoolOpts(o.DSN, o.DedicatedPool)

	return openShared(c, "queue", c.cfg.Queue.Adapter, []string{string(queue.DB)},
		key, backend, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (queue.Queue, error) {
			return queue.OpenShared(queue.Adapter(c.cfg.Queue.Adapter), conn, o)
		})
}

// openSharedSearch resolves the search service over the shared pool on an
// exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedSearch() (search.Search, bool, error) {
	o := c.cfg.Search.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)
	poolOpts := sharedPoolOpts(o.DSN, o.DedicatedPool)

	return openShared(c, "search", c.cfg.Search.Adapter,
		[]string{string(search.DB), string(search.Postgres), string(search.SQLite)},
		key, backend, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (search.Search, error) {
			return search.OpenShared(search.Adapter(c.cfg.Search.Adapter), conn, o)
		})
}

// openSharedSession resolves the session service over the shared pool on an
// exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedSession() (session.Store, bool, error) {
	o := c.cfg.Session.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)
	poolOpts := sharedPoolOpts(o.DSN, o.DedicatedPool)

	return openShared(c, "session", c.cfg.Session.Adapter, []string{"db"}, // "db" is sessiondb.Adapter; core/session defines no DB const.
		key, backend, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (session.Store, error) {
			return session.OpenShared(session.Adapter(c.cfg.Session.Adapter), conn, o)
		})
}

// openSharedIdempotency resolves the idempotency service over the shared
// pool on an exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedIdempotency() (idempotency.Store, bool, error) {
	o := c.cfg.Idempotency.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)
	poolOpts := sharedPoolOpts(o.DSN, o.DedicatedPool)

	return openShared(c, "idempotency", c.cfg.Idempotency.Adapter, []string{"db"}, // "db" is idemdb.Adapter; core/idempotency defines no DB const.
		key, backend, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (idempotency.Store, error) {
			return idempotency.OpenShared(idempotency.Adapter(c.cfg.Idempotency.Adapter), conn, o)
		})
}

// openSharedWorkflow resolves the workflow service over the shared pool on
// an exact postgres-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedWorkflow() (workflow.Workflow, bool, error) {
	o := c.cfg.Workflow.Options
	key, ok := postgresPoolKey(o.DSN)
	poolOpts := db.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}

	return openShared(c, "workflow", c.cfg.Workflow.Adapter, []string{string(workflow.DB), string(workflow.Postgres)},
		key, poolPostgres, ok, o.DedicatedPool, poolOpts, func(conn db.DB) (workflow.Workflow, error) {
			return workflow.OpenShared(workflow.Adapter(c.cfg.Workflow.Adapter), conn, o)
		})
}

// openSharedScheduler resolves the scheduler service over the shared pool
// on an exact-DSN match, injecting the resolved job dispatcher as today. It
// reports shared=false for the direct path.
func (c *Container) openSharedScheduler() (scheduler.Scheduler, bool, error) {
	opts := c.cfg.Scheduler.Options
	if opts.Dispatcher == nil {
		d, err := c.Job()
		if err != nil {
			return nil, true, fmt.Errorf("container: scheduler: resolve job: %w", err)
		}

		opts.Dispatcher = d
	}

	key, backend, ok := coreDSNPoolKey(opts.DSN)
	poolOpts := sharedPoolOpts(opts.DSN, opts.DedicatedPool)

	return openShared(c, "scheduler", c.cfg.Scheduler.Adapter, []string{"postgres"}, // "postgres" is schedpostgres.Adapter; core/scheduler defines no Postgres const.
		key, backend, ok, opts.DedicatedPool, poolOpts, func(conn db.DB) (scheduler.Scheduler, error) {
			return scheduler.OpenShared(scheduler.Adapter(c.cfg.Scheduler.Adapter), conn, opts)
		})
}

// openSharedVectorStore resolves the vectorstore service over the shared
// pool on an exact-DSN match. It reports shared=false for the direct path.
func (c *Container) openSharedVectorStore() (vectorstore.VectorStore, bool, error) {
	o := c.cfg.VectorStore.Options
	key, backend, ok := coreDSNPoolKey(o.DSN)

	return openShared(c, "vectorstore", c.cfg.VectorStore.Adapter,
		[]string{string(vectorstore.DB), string(vectorstore.PGVector), string(vectorstore.SQLite)},
		key, backend, ok, o.DedicatedPool, sharedPoolOpts(o.DSN, o.DedicatedPool),
		func(conn db.DB) (vectorstore.VectorStore, error) {
			return vectorstore.OpenShared(vectorstore.Adapter(c.cfg.VectorStore.Adapter), conn, o)
		})
}
