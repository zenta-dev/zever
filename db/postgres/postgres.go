package postgres

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zenta-dev/zever/db"
)

// pgPool is the narrow pool surface the adapter needs. *pgxpool.Pool
// satisfies it; tests inject fakes. (Mirrors vectorstore's dbpool pattern.)
type pgPool interface {
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
	Close()
}

var _ pgPool = (*pgxpool.Pool)(nil)

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// pgConn is the narrow connection surface pgTx pings. *pgx.Conn satisfies
// it; capturing it as an interface keeps pgTx.Ping unit-testable.
type pgConn interface {
	Ping(ctx context.Context) error
}

var _ pgConn = (*pgx.Conn)(nil)

// adapter deliberately does not implement db.Preparer. pgxpool already does
// transparent, per-connection prepared-statement caching by default: a
// pgxpool.Pool config that leaves QueryExecMode unset (as New's config below
// does -- it never sets config.ConnConfig.DefaultQueryExecMode) uses pgx's
// default of pgx.QueryExecModeCacheStatement, which prepares and caches a
// statement the first time its SQL text is seen on a given connection and
// reuses it on every subsequent Query/Exec with that same text on that
// connection. A second, adapter-level cache here would just duplicate work
// pgx already does.
type adapter struct {
	pool pgPool
}

func (a *adapter) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := a.pool.Query(ctx, replacePlaceholders(query), args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query error: %w", err)
	}

	return &rowsAdapter{rows: rows}, nil
}

func (a *adapter) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	tag, err := a.pool.Exec(ctx, replacePlaceholders(query), args...)
	if err != nil {
		return 0, fmt.Errorf("postgres: exec error: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (a *adapter) Ping(ctx context.Context) error {
	if err := a.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping error: %w", err)
	}

	return nil
}

// Dialect reports this adapter's registration name, matching db.Postgres.
func (a *adapter) Dialect() string { return "postgres" }

func (a *adapter) Close(ctx context.Context) error {
	ctxErr := ctx.Err()

	a.pool.Close()

	if ctxErr != nil {
		return fmt.Errorf("postgres: close error: %w", ctxErr)
	}

	return nil
}

func (a *adapter) BeginTx(ctx context.Context, opts *db.TxOptions) (db.Tx, error) {
	if opts == nil {
		opts = &db.TxOptions{}
	}

	var iso pgx.TxIsoLevel

	switch opts.Isolation {
	case db.ReadCommitted:
		iso = pgx.ReadCommitted
	case db.RepeatableRead:
		iso = pgx.RepeatableRead
	case db.Serializable:
		iso = pgx.Serializable
	default:
		iso = pgx.ReadCommitted
	}

	accessMode := pgx.ReadWrite
	if opts.ReadOnly {
		accessMode = pgx.ReadOnly
	}

	deferrableMode := pgx.NotDeferrable
	if opts.Deferrable {
		deferrableMode = pgx.Deferrable
	}

	pgOpts := pgx.TxOptions{
		IsoLevel:       iso,
		AccessMode:     accessMode,
		DeferrableMode: deferrableMode,
	}

	tx, err := a.pool.BeginTx(ctx, pgOpts)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin error: %w", err)
	}

	return &pgTx{tx: tx, conn: tx.Conn()}, nil
}

// pgTx implements db.Tx using pgx.
type pgTx struct {
	tx   pgx.Tx
	conn pgConn
	mu   sync.Mutex
	done bool
}

func (t *pgTx) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := t.tx.Query(ctx, replacePlaceholders(query), args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query error: %w", err)
	}

	return &rowsAdapter{rows: rows}, nil
}

func (t *pgTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	tag, err := t.tx.Exec(ctx, replacePlaceholders(query), args...)
	if err != nil {
		return 0, fmt.Errorf("postgres: exec error: %w", err)
	}

	return tag.RowsAffected(), nil
}

// Dialect reports this transaction's SQL dialect, matching the adapter it was opened from.
func (t *pgTx) Dialect() string { return "postgres" }

func (t *pgTx) Ping(ctx context.Context) error {
	t.mu.Lock()
	done := t.done
	t.mu.Unlock()

	if done {
		return errors.New("postgres: ping error: transaction already closed")
	}

	// The connection was captured at BeginTx; unit tests inject failures here.
	if err := t.conn.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping error: %w", err)
	}

	return nil
}

func (t *pgTx) Close(ctx context.Context) error {
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

func (t *pgTx) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return errors.New("postgres: commit error: transaction already closed")
	}

	if err := t.tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit error: %w", err)
	}

	t.done = true

	return nil
}

func (t *pgTx) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return nil
	}

	if err := t.tx.Rollback(ctx); err != nil {
		return fmt.Errorf("postgres: rollback error: %w", err)
	}

	t.done = true

	return nil
}

func (t *pgTx) Savepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}

	_, err := t.tx.Exec(ctx, "SAVEPOINT "+quoteIdent(name))
	if err != nil {
		return fmt.Errorf("postgres: savepoint error: %w", err)
	}

	return nil
}

func (t *pgTx) RollbackTo(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}

	_, err := t.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+quoteIdent(name))
	if err != nil {
		return fmt.Errorf("postgres: rollback to savepoint error: %w", err)
	}

	return nil
}

func validateSavepointName(name string) error {
	if name == "" {
		return fmt.Errorf("postgres: invalid savepoint name %q", name)
	}

	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return fmt.Errorf("postgres: invalid savepoint name %q", name)
			}
		} else {
			if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return fmt.Errorf("postgres: invalid savepoint name %q", name)
			}
		}
	}

	return nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

type rowsAdapter struct {
	rows pgx.Rows
}

func (r *rowsAdapter) Next() bool {
	return r.rows.Next()
}

func (r *rowsAdapter) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func (r *rowsAdapter) Close() error {
	r.rows.Close()

	if err := r.rows.Err(); err != nil {
		return fmt.Errorf("postgres: rows error: %w", err)
	}

	return nil
}

func (r *rowsAdapter) Err() error {
	return r.rows.Err()
}

// Columns returns the result set's column names, derived from pgx's field
// descriptions (pgx.Rows has no direct Columns method).
func (r *rowsAdapter) Columns() ([]string, error) {
	fields := r.rows.FieldDescriptions()

	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.Name
	}

	return cols, nil
}

// New creates a db.DB backed by PostgreSQL via pgxpool. The caller registers
// it via db.Register(db.Postgres, New). DSN is required; pool knobs from
// opts pass through to the pgxpool config. It never logs the DSN.
//
// Pool guidance: each adapter (db, search/postgres, vectorstore/pgvector)
// opens its own pool. When they share one Postgres DSN, keep the sum of
// per-adapter MaxConns below the server's max_connections; search and
// vectorstore default to 4 each (see their PoolConfig helpers).
func New(opts db.Options) (db.DB, error) {
	if strings.TrimSpace(opts.DSN) == "" {
		return nil, fmt.Errorf("postgres: option %q is required", "dsn")
	}

	config, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: config error: %w", err)
	}

	// Enforce a TLS 1.2 floor whenever TLS is in play. pgxpool.ParseConfig
	// already merges sslmode from BOTH the DSN string and the PGSSLMODE
	// environment variable into config.ConnConfig.TLSConfig, leaving it nil
	// only when TLS is truly disabled (DSN sslmode=disable or the
	// PGSSLMODE=disable env var) -- so checking the parsed config here
	// (instead of substring-matching the raw DSN text) covers the env-var
	// case the old DSN-only check missed. A nil TLSConfig is left alone as
	// explicit opt-out.
	if config.ConnConfig.TLSConfig != nil && config.ConnConfig.TLSConfig.MinVersion < tls.VersionTLS12 {
		config.ConnConfig.TLSConfig.MinVersion = tls.VersionTLS12
	}

	if opts.MaxConns > 0 {
		config.MaxConns = int32(opts.MaxConns) //nolint:gosec // G115: pool size bounded by design
	}

	if opts.MinConns > 0 {
		config.MinConns = int32(opts.MinConns) //nolint:gosec // G115: pool size bounded by design
	}

	if opts.MaxConnLifetime > 0 {
		config.MaxConnLifetime = opts.MaxConnLifetime
	}

	if opts.MaxConnIdleTime > 0 {
		config.MaxConnIdleTime = opts.MaxConnIdleTime
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	return &adapter{pool: pool}, nil
}
