package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "modernc.org/sqlite" // register sqlite driver

	"github.com/zenta-dev/zever/db"
)

// pragmaFK enforces foreign keys on every connection opened from the DSN.
const pragmaFK = "foreign_keys(1)"

// pragmaBusy sets the busy timeout to 5s on every connection opened from the DSN.
const pragmaBusy = "busy_timeout(5000)"

// sqlOpen opens a *sql.DB. It is a variable rather than a direct sql.Open
// call so tests can force the open-error branch, which is unreachable with
// the registered driver since sql.Open only fails for an unknown driver name.
var sqlOpen = sql.Open

var (
	_ db.DB         = (*adapter)(nil)
	_ db.Preparer   = (*adapter)(nil)
	_ db.Transactor = (*adapter)(nil)
	_ db.Stmt       = (*sqliteStmt)(nil)
	_ db.Tx         = (*sqliteTx)(nil)
	_ db.Rows       = (*rowsAdapter)(nil)
)

// adapter is a db.DB backed by database/sql over modernc.org/sqlite.
type adapter struct {
	conn      *sql.DB
	stmtCache *stmtCache
}

// New creates a db.DB backed by SQLite at opts.Path.
//
// An empty Path defaults to "app.db" in the process working directory; an
// explicitly blank (whitespace-only) Path is rejected. ":memory:" forces a
// single pooled connection, taking precedence over Options.MaxConns, since
// each additional connection to an in-memory database gets its own isolated
// database rather than sharing state. File-backed databases apply the pool
// knobs (MaxConns as max open, MinConns as max idle target, lifetimes).
//
// Error style: driver failures wrap as "sqlite: <op>: %w" so errors.Is/As
// keep working through the chain. Begin/Commit failures additionally wrap in
// *db.TxError (matching core's Op convention) even though db.WithTx wraps
// begin/commit once more on its own path; validation failures (bad path,
// bad savepoint name) are plain "sqlite: ...". No Path/DSN content is
// included in errors. Per-adapter sentinels were deliberately not added:
// core errors.go is owned by another agent and a parallel sentinel set
// would split error identity.
func New(opts db.Options) (db.DB, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		// Explicit whitespace-only path is an error; empty (zero-value) defaults to app.db.
		if opts.Path != "" {
			return nil, errors.New("sqlite: path must not be empty")
		}

		path = "app.db"
	}

	conn, err := sqlOpen("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	if path == ":memory:" || path == "file::memory:" {
		// An in-memory database (even shared-cache) is process-local state
		// tied to whichever connection(s) reference it; database/sql may
		// otherwise open a second pooled connection under concurrent access
		// (or transparently retry a statement on a fresh one after a
		// spurious bad-connection error), which can race independent DDL
		// against itself (observed as spurious "table already exists"
		// errors). A single connection removes that class of races. This
		// takes precedence over Options.MaxConns even if the caller set it,
		// since each additional connection to an in-memory database gets
		// its own isolated database rather than sharing state.
		conn.SetMaxOpenConns(1)
	} else {
		if opts.MaxConns > 0 {
			conn.SetMaxOpenConns(opts.MaxConns)
		}

		if opts.MinConns > 0 {
			conn.SetMaxIdleConns(opts.MinConns)
		}

		if opts.MaxConnLifetime > 0 {
			conn.SetConnMaxLifetime(opts.MaxConnLifetime)
		}

		if opts.MaxConnIdleTime > 0 {
			conn.SetConnMaxIdleTime(opts.MaxConnIdleTime)
		}
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(pingCtx); err != nil {
		// Best-effort close after ping failure; nothing actionable to report.
		_ = conn.Close()

		return nil, fmt.Errorf("sqlite: configure: %w", err)
	}

	return &adapter{
		conn:      conn,
		stmtCache: newStmtCache(defaultStmtCacheSize),
	}, nil
}

// Query runs a query and returns matching rows.
func (a *adapter) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := a.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}

	return &rowsAdapter{rows: rows}, nil
}

// Exec runs a query and returns the number of rows it affected.
func (a *adapter) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := a.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: exec: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: exec rows affected: %w", err)
	}

	return n, nil
}

// Ping verifies the connection is alive.
func (a *adapter) Ping(ctx context.Context) error {
	if err := a.conn.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite: ping: %w", err)
	}

	return nil
}

// Dialect reports this adapter's registration name, matching db.Register(db.SQLite, New).
func (a *adapter) Dialect() string { return db.SQLite.String() }

// Close releases database resources in order: statements, then the pool.
// Each stage is distinctly wrapped so callers can tell which failed.
func (a *adapter) Close(ctx context.Context) error {
	ctxErr := ctx.Err()
	stmtCloseErr := a.stmtCache.CloseAll()
	closeErr := a.conn.Close()

	if ctxErr != nil {
		return fmt.Errorf("sqlite: close: %w", ctxErr)
	}

	if stmtCloseErr != nil {
		return fmt.Errorf("sqlite: close statements: %w", stmtCloseErr)
	}

	if closeErr != nil {
		return fmt.Errorf("sqlite: close connection: %w", closeErr)
	}

	return nil
}

// Prepare implements db.Preparer with a cache-hit fast path. A *sql.Stmt
// returned by sql.DB.PrepareContext is not bound to one pooled connection --
// database/sql re-prepares it transparently on whichever connection a call
// ends up using -- so one cache per adapter (rather than per-connection) is
// correct.
func (a *adapter) Prepare(ctx context.Context, query string) (db.Stmt, error) {
	if stmt, ok := a.stmtCache.Get(query); ok {
		return &sqliteStmt{stmt: stmt}, nil
	}

	stmt, err := a.conn.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: prepare: %w", err)
	}

	// Put returns whichever statement the cache actually kept: our own if we
	// won the slot, or the previously-cached one if a concurrent Prepare of
	// the same query text won the race and closed ours as a redundant
	// duplicate. Returning the kept statement is what stops a race-loser
	// from wrapping an already-closed *sql.Stmt.
	stmt = a.stmtCache.Put(query, stmt)

	return &sqliteStmt{stmt: stmt}, nil
}

// BeginTx starts a transaction with the given options. A nil opts selects
// the default (read committed, read-write). Failures report as *db.TxError
// with Op "begin"; note db.WithTx wraps begin failures once more.
func (a *adapter) BeginTx(ctx context.Context, opts *db.TxOptions) (db.Tx, error) {
	if opts == nil {
		opts = &db.TxOptions{}
	}

	var level sql.IsolationLevel

	switch opts.Isolation {
	case db.ReadCommitted:
		level = sql.LevelReadCommitted
	case db.RepeatableRead:
		level = sql.LevelRepeatableRead
	case db.Serializable:
		level = sql.LevelSerializable
	default:
		level = sql.LevelReadCommitted
	}

	sqlOpts := &sql.TxOptions{
		Isolation: level,
		ReadOnly:  opts.ReadOnly,
	}

	tx, err := a.conn.BeginTx(ctx, sqlOpts)
	if err != nil {
		return nil, &db.TxError{Op: "begin", Err: fmt.Errorf("sqlite: begin: %w", err)}
	}

	return &sqliteTx{tx: tx}, nil
}

// sqliteStmt wraps a cached *sql.Stmt to implement db.Stmt. Close is a
// no-op: the statement is owned by the adapter's LRU cache and is closed
// only when evicted or when the adapter itself closes, so a caller done
// with one Prepare/Query-or-Exec round trip can Close it without
// invalidating it for the next caller that reuses the same query text.
type sqliteStmt struct {
	stmt *sql.Stmt
}

// Query runs the statement and returns matching rows.
func (s *sqliteStmt) Query(ctx context.Context, args ...any) (db.Rows, error) {
	rows, err := s.stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: stmt query: %w", err)
	}

	return &rowsAdapter{rows: rows}, nil
}

// Exec runs the statement and returns the number of rows it affected.
func (s *sqliteStmt) Exec(ctx context.Context, args ...any) (int64, error) {
	res, err := s.stmt.ExecContext(ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: stmt exec: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: stmt rows affected: %w", err)
	}

	return n, nil
}

// Close releases the prepared statement. It is a no-op (see sqliteStmt).
func (s *sqliteStmt) Close() error { return nil }

// sqliteTx implements db.Tx using database/sql. The mutex guards only the
// done flag; the underlying *sql.Tx handles its own concurrency.
type sqliteTx struct {
	tx   *sql.Tx
	mu   sync.Mutex
	done bool
}

// Query runs a query within the transaction and returns matching rows.
func (t *sqliteTx) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}

	return &rowsAdapter{rows: rows}, nil
}

// Exec runs a query within the transaction and returns rows affected.
func (t *sqliteTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: exec: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: exec rows affected: %w", err)
	}

	return n, nil
}

// Dialect reports this transaction's SQL dialect, matching the adapter it was opened from.
func (t *sqliteTx) Dialect() string { return db.SQLite.String() }

// Ping verifies the transaction is still open. sql.Tx has no Ping, so this
// reports the done flag instead of touching the database.
func (t *sqliteTx) Ping(ctx context.Context) error {
	_ = ctx

	t.mu.Lock()
	done := t.done
	t.mu.Unlock()

	if done {
		return errors.New("sqlite: ping: transaction already closed")
	}

	return nil
}

// Close rolls the transaction back if not yet committed; a closed
// transaction is a no-op.
func (t *sqliteTx) Close(ctx context.Context) error {
	t.mu.Lock()
	done := t.done
	t.mu.Unlock()

	if done {
		return nil
	}

	if err := t.Rollback(ctx); err != nil {
		return err
	}

	return nil
}

// Commit commits the transaction. It fails once-only: a second Commit
// reports *db.TxError with Op "commit" (as does a driver-level failure;
// note db.WithTx wraps commit failures once more).
func (t *sqliteTx) Commit(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return &db.TxError{Op: "commit", Err: errors.New("sqlite: commit: transaction already closed")}
	}

	if err := t.tx.Commit(); err != nil {
		return &db.TxError{Op: "commit", Err: fmt.Errorf("sqlite: commit: %w", err)}
	}

	t.done = true

	return nil
}

// Rollback aborts the transaction. It is idempotent: rolling back a closed
// transaction is a no-op returning nil.
func (t *sqliteTx) Rollback(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return nil
	}

	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rollback: %w", err)
	}

	t.done = true

	return nil
}

// Savepoint creates a named savepoint within the transaction. The name is
// validated before interpolation since identifiers cannot be bound args.
func (t *sqliteTx) Savepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}

	_, err := t.tx.ExecContext(ctx, "SAVEPOINT "+name)
	if err != nil {
		return fmt.Errorf("sqlite: savepoint: %w", err)
	}

	return nil
}

// RollbackTo rolls back to a named savepoint. The name is validated before
// interpolation since identifiers cannot be bound args.
func (t *sqliteTx) RollbackTo(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}

	_, err := t.tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
	if err != nil {
		return fmt.Errorf("sqlite: rollback to savepoint: %w", err)
	}

	return nil
}

// validateSavepointName rejects anything outside [A-Za-z_][A-Za-z0-9_]* so
// the name is safe to interpolate into SAVEPOINT statements.
func validateSavepointName(name string) error {
	if name == "" {
		return fmt.Errorf("sqlite: invalid savepoint name %q", name)
	}

	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return fmt.Errorf("sqlite: invalid savepoint name %q", name)
			}
		} else {
			if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return fmt.Errorf("sqlite: invalid savepoint name %q", name)
			}
		}
	}

	return nil
}

// rowsAdapter forwards Next/Scan/Close/Err to database/sql and wraps only
// Columns, the one call that can fail after iteration started.
type rowsAdapter struct {
	rows *sql.Rows
}

// Next advances to the next row, reporting whether one is available.
func (r *rowsAdapter) Next() bool {
	return r.rows.Next()
}

// Scan copies the current row into dest values.
func (r *rowsAdapter) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

// Close releases the rows iterator. It is idempotent.
func (r *rowsAdapter) Close() error {
	return r.rows.Close()
}

// Err reports the iteration error, if any.
func (r *rowsAdapter) Err() error {
	return r.rows.Err()
}

// Columns returns the result set column names.
func (r *rowsAdapter) Columns() ([]string, error) {
	cols, err := r.rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite: columns: %w", err)
	}

	return cols, nil
}

// buildDSN renders a driver DSN for path, enforcing foreign_keys and a 5s
// busy timeout unless the caller already set those pragmas, and forcing
// shared cache for in-memory databases.
func buildDSN(path string) string {
	if path == ":memory:" || path == "file::memory:" {
		return "file::memory:?cache=shared&_pragma=" + pragmaFK + "&_pragma=" + pragmaBusy
	}

	rawPath, rawQuery, hasQuery := strings.Cut(path, "?")
	prefix, pathBody := dsnPrefixAndBody(rawPath)
	q := dsnValues(rawQuery, hasQuery, pathBody)

	var b strings.Builder
	b.Grow(len(prefix) + len(pathBody) + len(rawQuery) + 64)
	b.WriteString(prefix)
	b.WriteString(pathBody)

	if len(q) > 0 {
		b.WriteByte('?')
		b.WriteString(q.Encode())
	}

	return b.String()
}

// dsnPrefixAndBody splits an explicit file: URI scheme from the path body.
// net/url mis-schemes a relative path that contains a colon (e.g. Windows
// drive or "log:2026.db") as a URI, so only "file:"-prefixed input is
// treated as URI; otherwise the path is escaped so '#' and '%' never act
// as URL structure.
func dsnPrefixAndBody(rawPath string) (string, string) {
	switch {
	case strings.HasPrefix(rawPath, "file://"):
		return "file://", escapePathChars(rawPath[len("file://"):])
	case strings.HasPrefix(rawPath, "file:"):
		return "file:", escapePathChars(rawPath[len("file:"):])
	default:
		return "file:", escapePathChars(rawPath)
	}
}

// dsnValues merges the caller's query params with the enforced pragmas,
// letting explicit user overrides (_pragma, _foreign_keys/_fk,
// _timeout) win over the defaults.
func dsnValues(rawQuery string, hasQuery bool, pathBody string) url.Values {
	q := url.Values{}

	if hasQuery {
		if parsed, perr := url.ParseQuery(rawQuery); perr == nil {
			q = parsed
		}
	}

	if pathBody == ":memory:" && !q.Has("cache") {
		q.Add("cache", "shared")
	}

	if !hasPragma(q, "foreign_keys") {
		q.Add("_pragma", pragmaFK)
	}

	if !hasPragma(q, "busy_timeout") {
		q.Add("_pragma", pragmaBusy)
	}

	return q
}

// escapePathChars percent-encodes path metacharacters via url.URL so that
// '#' and '%' in file names never act as URL structure.
func escapePathChars(path string) string {
	return (&url.URL{Path: path}).EscapedPath()
}

// hasPragma reports whether the query already sets pragma name, via any of
// the driver's spellings: _pragma=name(...), _name, or driver aliases
// (_fk for foreign_keys, _timeout for busy_timeout).
func hasPragma(q url.Values, name string) bool {
	if q.Has("_" + name) {
		return true
	}

	switch name {
	case "busy_timeout":
		if q.Has("_timeout") {
			return true
		}
	case "foreign_keys":
		if q.Has("_fk") {
			return true
		}
	}

	for _, v := range q["_pragma"] {
		key := v
		if i := strings.IndexAny(key, "(="); i >= 0 {
			key = key[:i]
		}

		if strings.EqualFold(strings.TrimSpace(key), name) {
			return true
		}
	}

	return false
}
