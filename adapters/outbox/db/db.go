package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// tsLayout is the fixed-width UTC timestamp layout used for sqlite cells so
// lexicographic ordering matches chronological ordering.
const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

// driver is a durable outbox.Store. It is safe for concurrent use.
type driver struct {
	conn       coredb.DB
	table      string
	inboxTable string
	poll       time.Duration
	batch      int
	// pubMu guards publisher, which application wiring attaches after Open
	// (see SetPublisher) while the relay goroutine reads it per message.
	pubMu       sync.RWMutex
	publisher   outbox.Publisher
	maxAttempts int
	retry       retryPolicy
	retention   time.Duration
	lock        time.Duration
	owns        bool
	recorder    outbox.Recorder

	// claimMu serializes sqlite claims: sqlite has no FOR UPDATE SKIP LOCKED,
	// and the adapter's process-local mutex replaces it.
	claimMu sync.Mutex

	// relayErr holds the most recent relay error for Status.
	relayErr atomic.Pointer[string]

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
	closed  bool
}

var (
	_ outbox.Store           = (*driver)(nil)
	_ outbox.Inbox           = (*driver)(nil)
	_ outbox.PublisherSetter = (*driver)(nil)
)

// retryPolicy narrows shared/retry.Policy to the one method the relay uses,
// keeping the driver testable without a full Policy.
type retryPolicy interface {
	NextDelay(attempt int) time.Duration
}

// New creates a DB-backed outbox store. Empty DSN selects sqlite at Path
// (default ":memory:"); a set DSN opens postgres. The outbox and inbox tables
// are created when missing.
func New(o Options) (outbox.Store, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}

	poolOpts := o.Options
	if strings.TrimSpace(poolOpts.DSN) == "" && poolOpts.Path == "" {
		poolOpts.Path = ":memory:"
	}

	var (
		conn coredb.DB
		err  error
	)

	if strings.TrimSpace(poolOpts.DSN) != "" {
		conn, err = dbpostgres.New(poolOpts)
	} else {
		conn, err = dbsqlite.New(poolOpts)
	}

	if err != nil {
		return nil, err
	}

	d, err := openFromDB(conn, o, true)
	if err != nil {
		_ = conn.Close(context.Background())

		return nil, err
	}

	return d, nil
}

// OpenFromDB creates a DB-backed outbox store over an already-open coredb.DB,
// skipping DSN/Path construction. The caller retains ownership of conn: Close
// on the returned Store does not close conn.
func OpenFromDB(conn coredb.DB, o Options) (outbox.Store, error) {
	if conn == nil {
		return nil, errors.New("db: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// NewFromDB creates a DB-backed outbox store over an already-open coredb.DB;
// see OpenFromDB.
func NewFromDB(conn coredb.DB, o Options) (outbox.Store, error) {
	return OpenFromDB(conn, o)
}

// openFromDB resolves defaults, pings conn, ensures the schema, and wires the
// driver. owns reports whether the driver owns conn and may close it.
func openFromDB(conn coredb.DB, o Options, owns bool) (outbox.Store, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	inboxTable := o.InboxTable
	if inboxTable == "" {
		inboxTable = DefaultInboxTable
	}

	poll := o.PollInterval
	if poll <= 0 {
		poll = DefaultPollInterval
	}

	batch := o.BatchSize
	if batch <= 0 {
		batch = DefaultBatchSize
	}

	maxAttempts := o.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}

	retention := o.Retention
	if retention <= 0 {
		retention = DefaultRetention
	}

	lockSeconds := o.LockSeconds
	if lockSeconds <= 0 {
		lockSeconds = DefaultLockSeconds
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	d := &driver{
		conn: conn, table: table, inboxTable: inboxTable, publisher: o.Publisher,
		poll: poll, batch: batch, maxAttempts: maxAttempts, retry: o.Retry,
		retention: retention, lock: time.Duration(lockSeconds) * time.Second,
		owns: owns, recorder: outbox.NewRecorder(o.Provider, string(outbox.DB), table, o.Transport),
	}

	if err := d.ensureSchema(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// checkDialect fails closed unless the connection's dialect is sqlite or
// postgres.
func (d *driver) checkDialect() error {
	switch d.conn.Dialect() {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("db: unsupported dialect %q", d.conn.Dialect())
	}
}

// ensureSchema creates the outbox and inbox tables and the outbox index when
// missing. DDL only: every state transition goes through parameterized SQL.
func (d *driver) ensureSchema(ctx context.Context) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	quoted := quoteIdent(d.table)
	inbox := quoteIdent(d.inboxTable)
	idx := quoteIdent(d.table + "_processed_idx")

	var ddl string

	if d.conn.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`id TEXT PRIMARY KEY, ` +
			`topic TEXT NOT NULL, ` +
			`"key" TEXT, ` +
			`payload BYTEA, ` +
			`headers TEXT, ` +
			`created_at TIMESTAMPTZ NOT NULL, ` +
			`attempts INTEGER NOT NULL DEFAULT 0, ` +
			`processed_at TIMESTAMPTZ, ` +
			`locked_until TIMESTAMPTZ, ` +
			`last_error TEXT, ` +
			`status TEXT NOT NULL DEFAULT 'pending')`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`id TEXT PRIMARY KEY, ` +
			`topic TEXT NOT NULL, ` +
			`"key" TEXT, ` +
			`payload BLOB, ` +
			`headers TEXT, ` +
			`created_at TEXT NOT NULL, ` +
			`attempts INTEGER NOT NULL DEFAULT 0, ` +
			`processed_at TEXT, ` +
			`locked_until TEXT, ` +
			`last_error TEXT, ` +
			`status TEXT NOT NULL DEFAULT 'pending')`
	}

	if _, err := d.conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("db: ensure schema: %w", err)
	}

	index := `CREATE INDEX IF NOT EXISTS ` + idx + ` ON ` + quoted + ` (processed_at, created_at)`
	if _, err := d.conn.Exec(ctx, index); err != nil {
		return fmt.Errorf("db: ensure schema: %w", err)
	}

	inboxDDL := `CREATE TABLE IF NOT EXISTS ` + inbox + ` (` +
		`event_id TEXT PRIMARY KEY, ` +
		`processed_at ` + timestampType(d.conn.Dialect()) + ` NOT NULL)`

	if _, err := d.conn.Exec(ctx, inboxDDL); err != nil {
		return fmt.Errorf("db: ensure schema: %w", err)
	}

	return nil
}

// timestampType returns the timestamp column type for dialect.
func timestampType(dialect string) string {
	if dialect == "postgres" {
		return "TIMESTAMPTZ"
	}

	return "TEXT"
}

// quoteIdent double-quotes a SQL identifier, escaping embedded quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ts renders t as a bind value: a time.Time for postgres, a fixed-width UTC
// string for sqlite.
func (d *driver) ts(t time.Time) any {
	if d.conn.Dialect() == "postgres" {
		return t.UTC()
	}

	return t.UTC().Format(tsLayout)
}

// Record persists msg in tx, the same transaction as the business write. A
// nil tx fails with ErrTxRequired. Trace context from ctx is injected into the
// stored headers so the relay can continue the originating trace.
func (d *driver) Record(ctx context.Context, tx coredb.Tx, msg outbox.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	if tx == nil {
		return fmt.Errorf("db: record: %w", outbox.ErrTxRequired)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	created := msg.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}

	headers := traceprop.Inject(ctx, msg.Headers)

	headersJSON := "{}"
	if len(headers) > 0 {
		buf, err := json.Marshal(headers)
		if err != nil {
			return fmt.Errorf("db: record %q: encode headers: %w", msg.ID, err)
		}

		headersJSON = string(buf)
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO `+quoteIdent(d.table)+` (id, topic, "key", payload, headers, created_at, attempts, status) `+
			`VALUES (?, ?, ?, ?, ?, ?, 0, 'pending')`,
		msg.ID, msg.Topic, msg.Key, msg.Payload, headersJSON, d.ts(created),
	)
	if err != nil {
		return fmt.Errorf("db: record %q: %w", msg.ID, err)
	}

	return nil
}

// Name returns the adapter name.
func (d *driver) Name() string { return "db" }
