package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// sweepInterval bounds how often Pop runs the reclaim sweep: an idle Pop
// otherwise issues a stale-claim UPDATE on every poll tick, which is
// unnecessary write load when nothing is stale. Gating the sweep to this
// cadence trades up to sweepInterval of extra recovery latency for a stale
// message for far fewer idle writes. Mirrors the redis adapter's sweep
// gate; PollInterval stays the knob for claim latency.
const sweepInterval = 250 * time.Millisecond

// msgRow is the queue_messages entity. Column order matches msgColumns:
// the positional Scan must read them in exactly this order.
type msgRow struct {
	Seq          int64
	MessageID    string
	Topic        string
	Payload      []byte
	Headers      string
	Attempt      int64
	AvailableAt  time.Time
	ClaimedBy    string
	ClaimedUntil time.Time
	CreatedAt    time.Time
}

// msgColumns is the entity column list in Scan order.
var msgColumns = []string{
	"id", "message_id", "topic", "payload", "headers",
	"attempt", "available_at", "claimed_by", "claimed_until", "created_at",
}

// Scan reads one row positionally, coercing driver representations:
// timestamps arrive as time.Time (postgres) or RFC3339Nano text (sqlite),
// integers arrive in any width.
func (r *msgRow) Scan(row orm.Row) error {
	var seqRaw, attemptRaw any
	var id, topic, owner string
	var payload []byte
	var headers string
	var availRaw, untilRaw, createdRaw any

	if err := row.Scan(&seqRaw, &id, &topic, &payload, &headers,
		&attemptRaw, &availRaw, &owner, &untilRaw, &createdRaw); err != nil {
		return err
	}

	seq, err := coerceInt(seqRaw)
	if err != nil {
		return fmt.Errorf("db: scan id: %w", err)
	}

	attempt, err := coerceInt(attemptRaw)
	if err != nil {
		return fmt.Errorf("db: scan attempt: %w", err)
	}

	avail, err := coerceTime(availRaw)
	if err != nil {
		return fmt.Errorf("db: scan available_at: %w", err)
	}

	until, err := coerceTime(untilRaw)
	if err != nil {
		return fmt.Errorf("db: scan claimed_until: %w", err)
	}

	created, err := coerceTime(createdRaw)
	if err != nil {
		return fmt.Errorf("db: scan created_at: %w", err)
	}

	*r = msgRow{
		Seq: seq, MessageID: id, Topic: topic, Payload: payload,
		Headers: headers, Attempt: attempt, AvailableAt: avail,
		ClaimedBy: owner, ClaimedUntil: until, CreatedAt: created,
	}

	return nil
}

// coerceTime converts a timestamp cell to time.Time.
func coerceTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t, nil
	case string:
		return time.Parse(time.RFC3339Nano, t)
	case []byte:
		return time.Parse(time.RFC3339Nano, string(t))
	default:
		return time.Time{}, fmt.Errorf("db: unsupported timestamp %T", v)
	}
}

// coerceInt converts an integer cell to int64.
func coerceInt(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("db: unsupported integer %T", v)
	}
}

// driver is a DB-backed queue.Queue with lease-based crash recovery. It is
// safe for concurrent use; all coordination goes through the database,
// never through in-process locks.
type driver struct {
	conn       coredb.DB
	tbl        orm.Table[msgRow]
	cSeq       orm.Column[msgRow, int64]
	cID        orm.Column[msgRow, string]
	cTopic     orm.Column[msgRow, string]
	cPayload   orm.Column[msgRow, []byte]
	cHeaders   orm.Column[msgRow, string]
	cAttempt   orm.Column[msgRow, int64]
	cAvail     orm.Column[msgRow, time.Time]
	cOwner     orm.Column[msgRow, string]
	cUntil     orm.Column[msgRow, time.Time]
	cCreated   orm.Column[msgRow, time.Time]
	tableName  string
	owner      string
	visibility time.Duration
	poll       time.Duration
	interval   time.Duration
	batch      int
	buffer     int
	owns       bool
	closed     atomic.Bool
	lastSweep  atomic.Int64
}

var _ queue.Queue = (*driver)(nil)

// New creates a DB-backed queue. Empty DSN selects sqlite at Path
// (default ":memory:"); a set DSN opens postgres. The messages table is
// created when missing. Note: ":memory:" sqlite uses shared cache, so two
// drivers with Path ":memory:" in one process share state; use distinct
// file paths for isolation.
func New(o Options) (queue.Queue, error) {
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

// NewFromDB creates a DB-backed queue over an already-open coredb.DB,
// skipping DSN/Path construction. The caller retains ownership of db:
// Close on the returned Queue does not close db, and a failed NewFromDB
// never closes db. Only sqlite and postgres dialects are supported;
// anything else fails closed.
func NewFromDB(conn coredb.DB, o Options) (queue.Queue, error) {
	if conn == nil {
		return nil, errors.New("db: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed queue over an already-open coredb.DB;
// see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (queue.Queue, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/owner/lease defaults, pings conn, ensures the
// schema, and wires the driver. owns reports whether the driver owns conn
// and may close it in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (queue.Queue, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	owner := o.Owner
	if owner == "" {
		owner = randomOwner()
	}

	visibility := o.VisibilityTimeout
	if visibility <= 0 {
		visibility = DefaultVisibilityTimeout
	}

	poll := o.PollTimeout
	if poll <= 0 {
		poll = DefaultPollTimeout
	}

	interval := o.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}

	batch := o.ReclaimBatch
	if batch <= 0 {
		batch = DefaultReclaimBatch
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectTimeout)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	d := &driver{
		conn: conn, tbl: orm.NewTable[msgRow](table, msgColumns),
		cSeq:      orm.NewColumn[msgRow, int64](table, "id"),
		cID:       orm.NewColumn[msgRow, string](table, "message_id"),
		cTopic:    orm.NewColumn[msgRow, string](table, "topic"),
		cPayload:  orm.NewColumn[msgRow, []byte](table, "payload"),
		cHeaders:  orm.NewColumn[msgRow, string](table, "headers"),
		cAttempt:  orm.NewColumn[msgRow, int64](table, "attempt"),
		cAvail:    orm.NewColumn[msgRow, time.Time](table, "available_at"),
		cOwner:    orm.NewColumn[msgRow, string](table, "claimed_by"),
		cUntil:    orm.NewColumn[msgRow, time.Time](table, "claimed_until"),
		cCreated:  orm.NewColumn[msgRow, time.Time](table, "created_at"),
		tableName: table, owner: owner, visibility: visibility,
		poll: poll, interval: interval, batch: batch,
		buffer: o.Buffer, owns: owns,
	}

	if err := d.ensureSchema(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// randomOwner mints a unique claim-holder name for one driver instance.
// crypto/rand failure is impossible to surface usefully here, so a short
// fallback keeps construction infallible.
func randomOwner() string {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "queue-owner-fallback"
	}

	return "queue-owner-" + hex.EncodeToString(b[:])
}

// checkDialect fails closed on dialects outside sqlite/postgres.
func (d *driver) checkDialect() error {
	switch d.conn.Dialect() {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("orm: db: unsupported dialect %q: %w",
			d.conn.Dialect(), dialect.ErrUnsupportedByDialect)
	}
}

// ensureSchema creates the messages table and topic index when missing.
// DDL only: every state transition below goes through the orm typed
// builder. The autoincrement seq column orders FIFO: Postgres BIGSERIAL
// and sqlite AUTOINCREMENT both assign insertion order.
func (d *driver) ensureSchema(ctx context.Context) error {
	if err := d.checkDialect(); err != nil {
		return err
	}

	quoted := `"` + strings.ReplaceAll(d.tableName, `"`, `""`) + `"`
	idx := `"` + strings.ReplaceAll(d.tableName+"_topic_idx", `"`, `""`) + `"`

	var ddl, index string

	if d.conn.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`id BIGSERIAL PRIMARY KEY, ` +
			`message_id TEXT NOT NULL UNIQUE, ` +
			`topic TEXT NOT NULL, ` +
			`payload BYTEA NOT NULL, ` +
			`headers TEXT NOT NULL, ` +
			`attempt BIGINT NOT NULL DEFAULT 1, ` +
			`available_at TIMESTAMPTZ NOT NULL, ` +
			`claimed_by TEXT NOT NULL DEFAULT '', ` +
			`claimed_until TIMESTAMPTZ NOT NULL, ` +
			`created_at TIMESTAMPTZ NOT NULL)`
		index = `CREATE INDEX IF NOT EXISTS ` + idx + ` ON ` + quoted + ` (topic)`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`id INTEGER PRIMARY KEY AUTOINCREMENT, ` +
			`message_id TEXT NOT NULL UNIQUE, ` +
			`topic TEXT NOT NULL, ` +
			`payload BLOB NOT NULL, ` +
			`headers TEXT NOT NULL, ` +
			`attempt INTEGER NOT NULL DEFAULT 1, ` +
			`available_at TEXT NOT NULL, ` +
			`claimed_by TEXT NOT NULL DEFAULT '', ` +
			`claimed_until TEXT NOT NULL, ` +
			`created_at TEXT NOT NULL)`
		index = `CREATE INDEX IF NOT EXISTS ` + idx + ` ON ` + quoted + ` (topic)`
	}

	if _, err := d.conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("db: ensure schema: %w", err)
	}

	if _, err := d.conn.Exec(ctx, index); err != nil {
		return fmt.Errorf("db: ensure schema: %w", err)
	}

	return nil
}

// encodeHeaders serializes headers to the TEXT cell. encoding/json cannot
// fail on map[string]string, so the error is provably infallible and
// discarded.
func encodeHeaders(h queue.Headers) string {
	buf, _ := json.Marshal(map[string]string(h))

	return string(buf)
}

// decodeHeaders parses a TEXT cell back to Headers. Null, empty, and
// corrupt cells decode to nil rather than failing the Pop: the payload
// is the contract, headers are metadata.
func decodeHeaders(s string) queue.Headers {
	if s == "" {
		return nil
	}

	var m map[string]string

	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}

	return queue.Headers(m)
}

// rowToMessage converts a claimed row to the public Message shape.
func rowToMessage(row *msgRow, topic string) (queue.Message, error) {
	id, err := queue.ParseMessageID(row.MessageID)
	if err != nil {
		return queue.Message{}, err
	}

	return queue.Message{
		ID:      id,
		Topic:   topic,
		Payload: queue.Payload(append([]byte(nil), row.Payload...)),
		Headers: decodeHeaders(row.Headers),
		Attempt: int(row.Attempt),
	}, nil
}

// Push enqueues a message on topic immediately claimable.
func (d *driver) Push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers) error {
	return d.push(ctx, topic, payload, headers, 0)
}

// PushDelayed enqueues a message on topic claimable after delay.
// Non-positive delays are immediately claimable.
func (d *driver) PushDelayed(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	return d.push(ctx, topic, payload, headers, delay)
}

func (d *driver) push(ctx context.Context, topic string, payload queue.Payload, headers queue.Headers, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return queue.ErrClosed
	}

	if err := d.checkDialect(); err != nil {
		return err
	}

	if d.buffer > 0 {
		if delay <= 0 {
			if err := d.waitForSpace(ctx, topic, d.countReady); err != nil {
				return err
			}
		} else if err := d.waitForSpace(ctx, topic, d.countUnclaimed); err != nil {
			return err
		}
	}

	if d.closed.Load() {
		return queue.ErrClosed
	}

	msg := queue.NewMessage(topic, payload, traceprop.Inject(ctx, headers))
	now := time.Now().UTC()

	available := now
	if delay > 0 {
		available = now.Add(delay)
	}

	err := orm.InsertInto(d.tbl).Values(
		orm.Set(d.cID, msg.ID.String()),
		orm.Set(d.cTopic, topic),
		orm.Set(d.cPayload, []byte(msg.Payload)),
		orm.Set(d.cHeaders, encodeHeaders(msg.Headers)),
		orm.Set(d.cAttempt, int64(1)),
		orm.Set(d.cAvail, available),
		orm.Set(d.cOwner, ""),
		orm.Set(d.cUntil, now),
		orm.Set(d.cCreated, now),
	).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("db: push %q: %w", topic, err)
	}

	return nil
}

// waitForSpace blocks while count reaches the buffer cap, polling on the
// Pop retry cadence so Push stays responsive to cancellation.
func (d *driver) waitForSpace(ctx context.Context, topic string, count func(context.Context, string) (int64, error)) error {
	tick := time.NewTicker(d.interval)
	defer tick.Stop()

	for {
		n, err := count(ctx, topic)
		if err != nil {
			return err
		}

		if n < int64(d.buffer) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("db: wait for buffer space: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

// countReady counts claimable rows in topic: available and not live-claimed.
// Stale claims are excluded until the reclaim sweep releases them, so
// Length counts ready rows only, matching the redis adapter.
func (d *driver) countReady(ctx context.Context, topic string) (int64, error) {
	n, err := orm.From[msgRow, *msgRow](d.tbl).Where(orm.And(
		d.cTopic.Eq(topic),
		d.cAvail.Lte(time.Now().UTC()),
		d.cOwner.Eq(""),
	)).Count(ctx, d.conn)
	if err != nil {
		return 0, fmt.Errorf("db: length %q: %w", topic, err)
	}

	return n, nil
}

// countUnclaimed counts every unclaimed row in topic (ready plus delayed),
// mirroring the redis adapter's delayed-space check.
func (d *driver) countUnclaimed(ctx context.Context, topic string) (int64, error) {
	n, err := orm.From[msgRow, *msgRow](d.tbl).Where(orm.And(
		d.cTopic.Eq(topic),
		d.cOwner.Eq(""),
	)).Count(ctx, d.conn)
	if err != nil {
		return 0, fmt.Errorf("db: length %q: %w", topic, err)
	}

	return n, nil
}

// Pop dequeues the oldest ready message from topic, holding a visibility
// lease for VisibilityTimeout. It sweeps stale claims and retries claim
// until PollTimeout lapses with EmptyError, mirroring the redis popLoop
// sweep order (reclaimStale, then tryClaim). Delayed rows are excluded by
// the available_at predicate, so no separate promotion pass is needed.
func (d *driver) Pop(ctx context.Context, topic string) (queue.Message, error) {
	if d.closed.Load() {
		return queue.Message{}, queue.ErrClosed
	}

	if err := ctx.Err(); err != nil {
		return queue.Message{}, fmt.Errorf("db: pop cancelled: %w", err)
	}

	if err := d.checkDialect(); err != nil {
		return queue.Message{}, err
	}

	poll := time.NewTimer(d.poll)
	defer poll.Stop()

	tick := time.NewTicker(d.interval)
	defer tick.Stop()

	if err := d.reclaimStale(ctx, topic); err != nil {
		return queue.Message{}, err
	}

	d.lastSweep.Store(time.Now().UnixMilli())

	for {
		msg, ok, err := d.tryClaim(ctx, topic)
		if err != nil {
			return queue.Message{}, err
		}

		if ok {
			return msg, nil
		}

		select {
		case <-ctx.Done():
			return queue.Message{}, fmt.Errorf("db: pop cancelled: %w", ctx.Err())
		case <-poll.C:
			return queue.Message{}, &queue.EmptyError{Topic: topic}
		case <-tick.C:
			now := time.Now().UnixMilli()
			if last := d.lastSweep.Load(); now-last >= sweepInterval.Milliseconds() {
				if d.lastSweep.CompareAndSwap(last, now) {
					if err := d.reclaimStale(ctx, topic); err != nil {
						return queue.Message{}, err
					}
				}
			}
		}
	}
}

// tryClaim adopts the oldest ready row in topic with a compare-and-set
// UPDATE guarded on the previously read lease, exactly like
// workflow/postgres's reclaim CAS and the redis claim.lua pop. Contended
// rows report not-claimed so Pop retries; only sqlite and postgres reach
// here, so no row-locking variant is needed (sqlite has no FOR UPDATE).
func (d *driver) tryClaim(ctx context.Context, topic string) (queue.Message, bool, error) {
	now := time.Now().UTC()

	row, ok, err := orm.From[msgRow, *msgRow](d.tbl).Where(orm.And(
		d.cTopic.Eq(topic),
		d.cAvail.Lte(now),
		d.cOwner.Eq(""),
	)).OrderBy(d.cSeq.Asc()).Limit(1).First(ctx, d.conn)
	if err != nil {
		return queue.Message{}, false, fmt.Errorf("db: pop %q: %w", topic, err)
	}

	if !ok {
		return queue.Message{}, false, nil
	}

	deadline := time.Now().UTC().Add(d.visibility)

	n, err := orm.UpdateTable(d.tbl).Where(orm.And(
		d.cID.Eq(row.MessageID),
		d.cOwner.Eq(""),
		d.cAttempt.Eq(row.Attempt),
	)).Set(
		orm.Set(d.cOwner, d.owner),
		orm.Set(d.cUntil, deadline),
	).Exec(ctx, d.conn)
	if err != nil {
		return queue.Message{}, false, fmt.Errorf("db: pop %q: %w", topic, err)
	}

	if n == 0 {
		return queue.Message{}, false, nil
	}

	msg, err := rowToMessage(row, topic)
	if err != nil {
		return queue.Message{}, false, err
	}

	return msg, true, nil
}

// reclaimStale releases expired claims in topic back to ready with
// attempt+1, mirroring reclaim.lua and the workflow attempt bump. Each
// row settles under its own lease guard, so a concurrent claim winning
// the row skips it instead of double-bumping. At most batch rows release
// per sweep.
func (d *driver) reclaimStale(ctx context.Context, topic string) error {
	now := time.Now().UTC()

	rows, err := orm.From[msgRow, *msgRow](d.tbl).Where(orm.And(
		d.cTopic.Eq(topic),
		d.cOwner.Neq(""),
		d.cUntil.Lte(now),
	)).OrderBy(d.cSeq.Asc()).Limit(d.batch).All(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("db: reclaim %q: %w", topic, err)
	}

	for _, row := range rows {
		_, uerr := orm.UpdateTable(d.tbl).Where(orm.And(
			d.cID.Eq(row.MessageID),
			d.cOwner.Eq(row.ClaimedBy),
			d.cAttempt.Eq(row.Attempt),
		)).Set(
			orm.Set(d.cAttempt, row.Attempt+1),
			orm.Set(d.cOwner, ""),
			orm.Set(d.cUntil, time.Now().UTC()),
		).Exec(ctx, d.conn)
		if uerr != nil {
			return fmt.Errorf("db: reclaim %q: %w", topic, uerr)
		}
	}

	return nil
}

// Ack acknowledges successful processing, deleting the row. The (id,
// attempt) guard makes Ack idempotent and stale-safe: a row reclaimed and
// redelivered after this Message was popped no longer matches, so a late
// Ack cannot delete another delivery. Missing rows stay nil, matching the
// redis ack.lua miss behavior.
func (d *driver) Ack(ctx context.Context, msg queue.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return queue.ErrClosed
	}

	if err := d.checkDialect(); err != nil {
		return err
	}

	_, err := orm.DeleteFrom(d.tbl).Where(orm.And(
		d.cID.Eq(msg.ID.String()),
		d.cAttempt.Eq(int64(msg.Attempt)),
	)).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("db: ack: %w", err)
	}

	return nil
}

// Nack reports processing failure. With requeue the row returns to ready
// with attempt+1 under the (id, attempt) guard, without touching the
// stored payload or headers; without requeue the row drops. Both are
// no-ops when the row already settled, matching nack.lua.
func (d *driver) Nack(ctx context.Context, msg queue.Message, requeue bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return queue.ErrClosed
	}

	if err := d.checkDialect(); err != nil {
		return err
	}

	id := msg.ID.String()
	attempt := int64(msg.Attempt)

	if !requeue {
		_, err := orm.DeleteFrom(d.tbl).Where(orm.And(
			d.cID.Eq(id),
			d.cAttempt.Eq(attempt),
		)).Exec(ctx, d.conn)
		if err != nil {
			return fmt.Errorf("db: nack: %w", err)
		}

		return nil
	}

	_, err := orm.UpdateTable(d.tbl).Where(orm.And(
		d.cID.Eq(id),
		d.cAttempt.Eq(attempt),
	)).Set(
		orm.Set(d.cAttempt, attempt+1),
		orm.Set(d.cOwner, ""),
		orm.Set(d.cUntil, time.Now().UTC()),
		orm.Set(d.cAvail, time.Now().UTC()),
	).Exec(ctx, d.conn)
	if err != nil {
		return fmt.Errorf("db: nack: %w", err)
	}

	return nil
}

// Length returns the number of ready messages in topic: available and not
// live-claimed. Inflight and delayed rows are excluded.
func (d *driver) Length(ctx context.Context, topic string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if d.closed.Load() {
		return 0, queue.ErrClosed
	}

	if err := d.checkDialect(); err != nil {
		return 0, err
	}

	return d.countReady(ctx, topic)
}

// IsEmpty reports whether topic contains no ready messages.
func (d *driver) IsEmpty(ctx context.Context, topic string) (bool, error) {
	n, err := d.Length(ctx, topic)

	return n == 0, err
}

// Close shuts down the queue; it is idempotent. Drivers built via New own
// their connection and release it; drivers built via NewFromDB borrow the
// caller's DB and leave it open.
func (d *driver) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if !d.owns {
		return nil
	}

	return d.conn.Close(context.Background())
}

// Name returns the adapter name for the queue.
func (d *driver) Name() string { return "db" }
