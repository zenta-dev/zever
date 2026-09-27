package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"

	whttp "github.com/zenta-dev/zever/adapters/webhook/http"
	"github.com/zenta-dev/zever/core/webhook"
)

// schema is applied in a single Exec at Open time.
const schema = `CREATE TABLE IF NOT EXISTS webhook_targets (
	event TEXT NOT NULL,
	target TEXT NOT NULL,
	secret TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (event, target)
)`

var memoryCounter uint64

// DefaultOpenTimeout bounds SQLite ping, schema setup, and registration reload.
const DefaultOpenTimeout = 5 * time.Second

// adapter is a durable registration store (SQLite) fronting the synchronous
// delivery engine composed from the http adapter. It holds no delivery logic:
// Register validation and Deliver behave exactly as the http adapter defines.
type adapter struct {
	db    *sql.DB
	inner webhook.Webhook
	dsn   string
}

// New builds a durable Webhook from o.
//
// It validates o, builds the http delivery engine first (so timeout/retry
// misconfiguration fails before any storage is touched), then opens the
// SQLite store and reloads every persisted registration into the engine.
// A stored row that no longer validates (for example a private target under
// changed options) fails New loudly instead of being silently dropped.
func New(o webhook.Options) (webhook.Webhook, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	// whttp.New runs the identical core Validate already passed above, so
	// its failure branch is unreachable in practice and intentionally left
	// without a dedicated test.
	inner, err := whttp.New(o)
	if err != nil {
		return nil, err
	}

	db, dsn, err := openDB(o.DSN)
	if err != nil {
		_ = inner.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	a := &adapter{db: db, inner: inner, dsn: dsn}

	if err := a.load(); err != nil {
		_ = inner.Close()
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	return a, nil
}

// openDB opens the SQLite store for dsn. An empty dsn (or the ":memory:"
// alias) maps to a unique shared-cache memory database so parallel adapters
// never share registrations. sql.Open is lazy, so PingContext fails fast on
// bad paths before any DDL runs.
func openDB(dsn string) (*sql.DB, string, error) {
	if dsn == "" || dsn == ":memory:" {
		n := atomic.AddUint64(&memoryCounter, 1)
		dsn = fmt.Sprintf("file:zen-webhook-%d?mode=memory&cache=shared", n)
	} else if err := validateDSN(dsn); err != nil {
		return nil, "", err
	}

	// NewConnector parses the DSN eagerly, so a malformed DSN fails here
	// with a coverable error; sql.Open would defer every failure to first
	// use and leave this branch untestable.
	connector, err := sqlite.NewConnector(dsn)
	if err != nil {
		return nil, "", fmt.Errorf("sqlite: open: %w", err)
	}

	db := sql.OpenDB(connector)

	// A single pooled connection serializes access at the Go level. This
	// keeps :memory: shared-cache databases (process-local state tied to
	// whichever connection references them) from racing DDL against
	// themselves on a second pooled connection, and avoids SQLITE_BUSY on
	// concurrent writes without a busy-timeout pragma.
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultOpenTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, "", err
	}

	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, "", err
	}

	return db, dsn, nil
}

// load replays every persisted registration into the delivery engine. The
// engine re-validates each target, so stale rows fail Open instead of
// vanishing silently.
func (a *adapter) load() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultOpenTimeout)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `SELECT event, target, secret FROM webhook_targets`)
	if err != nil {
		return err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var event, target, secret string
		if err := rows.Scan(&event, &target, &secret); err != nil {
			_ = rows.Close()
			return err
		}

		if err := a.inner.Register(ctx, event, target, secret); err != nil {
			_ = rows.Close()
			return fmt.Errorf("load %q %q: %w", event, target, err)
		}
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}

	return nil
}

// Register validates via the delivery engine first, then persists with
// INSERT OR REPLACE. If persistence fails after the engine already
// registered, the engine registration is rolled back so the two never
// diverge.
func (a *adapter) Register(ctx context.Context, event, target, secret string) error {
	if err := a.inner.Register(ctx, event, target, secret); err != nil {
		return err
	}

	if _, err := a.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO webhook_targets (event, target, secret) VALUES (?, ?, ?)`,
		event, target, secret,
	); err != nil {
		_ = a.inner.Unregister(ctx, event, target)
		return fmt.Errorf("sqlite: register: %w", err)
	}

	return nil
}

// Unregister removes the engine subscription first (surfacing NotFound), then
// deletes the persisted row. A missing row after a successful engine removal
// stays nil for idempotency, matching http semantics.
func (a *adapter) Unregister(ctx context.Context, event, target string) error {
	if err := a.inner.Unregister(ctx, event, target); err != nil {
		return err
	}

	if _, err := a.db.ExecContext(ctx,
		`DELETE FROM webhook_targets WHERE event = ? AND target = ?`,
		event, target,
	); err != nil {
		return fmt.Errorf("sqlite: unregister: %w", err)
	}

	return nil
}

// Deliver fans payload out to every target subscribed to event.
func (a *adapter) Deliver(ctx context.Context, event string, payload []byte) error {
	return a.inner.Deliver(ctx, event, payload)
}

// Close releases the delivery engine and the database, joining both errors.
func (a *adapter) Close() error {
	return errors.Join(a.inner.Close(), a.db.Close())
}

// validateDSN rejects control characters and statement separators that would
// let a DSN smuggle extra SQLite pragmas or statements. It duplicates the
// vectorstore sqlite check instead of sharing it: webhook must not depend on
// an unrelated facade, and the ten-line check is cheaper than the coupling.
func validateDSN(dsn string) error {
	if strings.Contains(dsn, "\x00") || strings.Contains(dsn, "\n") || strings.Contains(dsn, "\r") {
		return errors.New("sqlite: dsn contains invalid control characters")
	}

	if strings.Contains(dsn, ";") {
		return errors.New("sqlite: dsn must not contain ';'")
	}

	return nil
}
