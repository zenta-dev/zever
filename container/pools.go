package container

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zenta-dev/zever/core/db"
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

// PoolMismatch records one borrower whose nonzero MaxConns differed from the
// shared pool's: the borrower's cap was ignored in favor of the first
// opener's. It carries no DSN or path content, only the service name and
// the two caps, so it is safe to surface in diagnostics output.
type PoolMismatch struct {
	Service string
	Want    int
	Have    int
}

// PoolInfo describes one resolved shared pool for diagnostics: its backend,
// the services borrowing it, and the effective MaxConns (the first opener's
// knobs). It carries no DSN or path content.
type PoolInfo struct {
	Backend  string
	Services []string
	MaxConns int
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
//
// Sharing rule: the registry key is the normalized DSN alone (exact string
// for postgres, cleaned path for sqlite), so the same DSN always shares
// one pool no matter which service or instance borrows it. Ref identity
// (service/name, see provide.go) is attribution, not keying: every
// successful borrow records the borrowing service for diagnostics
// (PoolStatus, PoolMismatches) and Close dedup, but never splits a pool.
// :memory: and empty DSNs never share (see isMemorySQLite,
// coreDSNPoolKey).
type poolRegistry struct {
	mu    sync.Mutex
	pools map[string]*poolEntry
	// mismatchLog records every MaxConns mismatch warn in borrow order, so
	// diagnostics (doctor) can surface them after the fact. It carries no
	// DSN content (see PoolMismatch).
	mismatchLog []PoolMismatch
	// open builds a fresh pool. Production resolves via the db registry so
	// the container wires no adapters itself; tests inject a fake.
	open func(adapter db.Adapter, opts db.Options) (db.DB, error)
	// warn reports pool lifecycle notes (shared opens, cap mismatches,
	// dedicated opens). Default writes one line to stderr without DSN
	// content; tests inject a recorder.
	warn func(msg string)
}

// newPoolRegistry builds an empty registry with production defaults. The
// pools map is left nil: borrow lazily creates it on first use, so a
// Container that never resolves a shareable DSN allocates nothing for it.
func newPoolRegistry() *poolRegistry {
	return &poolRegistry{
		open: db.Open,
		warn: defaultPoolWarn,
	}
}

// poolInfoPrefix marks the purely informational "pool shared ..." lines
// emitted by logOpen. They are debug-level: shown on stderr only when the
// ZEVER_DEBUG environment variable is non-empty. Mismatch warnings (which
// signal ignored configuration) are always shown.
const poolInfoPrefix = "container: pool shared "

// defaultPoolWarn writes pool lifecycle notes to stderr, dropping the
// informational shared-open lines unless ZEVER_DEBUG is set.
func defaultPoolWarn(msg string) {
	if strings.HasPrefix(msg, poolInfoPrefix) && os.Getenv("ZEVER_DEBUG") == "" {
		return
	}

	fmt.Fprintln(os.Stderr, msg)
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
			r.logOpen(e.backend, key, services, knobs)

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
		mismatch := opts.MaxConns != 0 && opts.MaxConns != knobs.MaxConns
		r.mu.Unlock()
		if mismatch {
			r.mu.Lock()
			r.mismatchLog = append(r.mismatchLog, PoolMismatch{Service: service, Want: opts.MaxConns, Have: knobs.MaxConns})
			r.mu.Unlock()
			r.warn(fmt.Sprintf("container: %s (shared pool): max_conns=%d ignored, shared pool max_conns=%d",
				service, opts.MaxConns, knobs.MaxConns))
		}
		r.logOpen(e.backend, key, services, knobs)

		return conn, nil
	}
}

// logOpen emits one observability line per shared-pool open or join. The
// line carries only the parsed host/dbname (postgres) or the backend
// (sqlite): full DSNs and file paths never appear.
func (r *poolRegistry) logOpen(backend, key string, services []string, knobs db.Options) {
	if backend == poolPostgres {
		preview := postgresPreview(strings.TrimPrefix(key, "postgres:"))
		r.warn(fmt.Sprintf("container: pool shared postgres %s max_conns=%d services=[%s]",
			preview, knobs.MaxConns, strings.Join(services, ",")))

		return
	}

	r.warn(fmt.Sprintf("container: pool shared sqlite max_conns=%d services=[%s]",
		knobs.MaxConns, strings.Join(services, ",")))
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

// mismatches returns every recorded MaxConns mismatch in borrow order.
// The result is a copy: callers may retain it without holding the lock.
func (r *poolRegistry) mismatches() []PoolMismatch {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]PoolMismatch(nil), r.mismatchLog...)
}

// status describes every resolved (non-building) pool in deterministic key
// order for diagnostics. It carries no DSN or path content: only the
// backend, borrowing services, and effective cap.
func (r *poolRegistry) status() []PoolInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]string, 0, len(r.pools))
	for k, e := range r.pools {
		if !e.building {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	out := make([]PoolInfo, 0, len(keys))
	for _, k := range keys {
		e := r.pools[k]
		out = append(out, PoolInfo{
			Backend:  e.backend,
			Services: append([]string(nil), e.services...),
			MaxConns: e.knobs.MaxConns,
		})
	}

	return out
}

// String renders m as one diagnostics line naming the borrower, the
// ignored cap, and the effective shared cap, with the dedicated_pool fix
// hint. It carries no DSN content.
func (m PoolMismatch) String() string {
	return fmt.Sprintf("pool: %s wants max_conns=%d but shares pool max_conns=%d (first opener wins) — set dedicated_pool: true to isolate",
		m.Service, m.Want, m.Have)
}

// PoolMismatches returns every shared-pool MaxConns mismatch recorded so
// far, in borrow order. A mismatch means the borrower's nonzero MaxConns
// was ignored in favor of the first opener's: the pool runs with one cap
// for all borrowers. Call after resolving services; an empty result means
// no borrower disagreed. The result is a copy safe to retain.
func (c *Container) PoolMismatches() []PoolMismatch {
	return c.pools.mismatches()
}

// PoolStatus describes every resolved shared pool in deterministic order
// for diagnostics: backend, borrowing services, and effective MaxConns. It
// carries no DSN or path content. Pools resolve lazily, so call after
// resolving services.
func (c *Container) PoolStatus() []PoolInfo {
	return c.pools.status()
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
