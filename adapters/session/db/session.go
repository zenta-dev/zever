package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/shared/codec"
	"github.com/zenta-dev/zever/shared/kvstore"
)

// Compile-time check that driver implements session.Store.
var _ session.Store = (*driver)(nil)

// sessionCodec (de)serializes wireSession records stored per key.
var sessionCodec = codec.JSONCodec[wireSession]{}

// wireSession is the JSON value stored per key. Times are Unix nanoseconds
// (0 means zero time); Data round-trips through JSON, so numbers decode as
// float64 and fresh maps are produced on every read (ownership is clean).
type wireSession struct {
	Data      map[string]any `json:"data"`
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
	// ExpiresAt is an int64 unixnano wire field.
	ExpiresAt int64 `json:"expires_at"`
}

// driver is a DB-backed session.Store. It is safe for concurrent use.
type driver struct {
	db     coredb.DB
	kv     *kvstore.Store
	prefix string
	ttl    time.Duration
	owns   bool
	closed atomic.Bool
}

// New creates a DB-backed session.Store. Empty DSN selects sqlite at Path
// (default ":memory:"); a set DSN opens postgres. The sessions table is
// created when missing. The driver owns its connection: Close releases it.
func New(o Options) (session.Store, error) {
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

// NewFromDB creates a DB-backed session.Store over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned Store does not close db, and a failed
// NewFromDB never closes db.
func NewFromDB(conn coredb.DB, o Options) (session.Store, error) {
	if conn == nil {
		return nil, errors.New("db: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed session.Store over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (session.Store, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/prefix/ttl defaults, opens the kv store, and
// wires the driver. owns reports whether the driver owns conn and may
// close it in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (session.Store, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	prefix := o.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}

	ttl := o.TTL
	if ttl == 0 {
		ttl = session.DefaultTTL
	}

	kv, err := kvstore.New(conn, table)
	if err != nil {
		return nil, err
	}

	return &driver{db: conn, kv: kv, prefix: prefix, ttl: ttl, owns: owns}, nil
}

func (d *driver) key(id string) string {
	return d.prefix + ":" + id
}

func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}

	return t.UnixNano()
}

func fromNano(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}

	return time.Unix(0, n).UTC()
}

func toWire(sess session.Session) wireSession {
	return wireSession{
		Data:      sess.Data,
		CreatedAt: unixNano(sess.CreatedAt),
		UpdatedAt: unixNano(sess.UpdatedAt),
		ExpiresAt: unixNano(sess.ExpiresAt),
	}
}

func fromWire(id string, w wireSession) session.Session {
	return session.Session{
		ID:        id,
		Data:      w.Data,
		CreatedAt: fromNano(w.CreatedAt),
		UpdatedAt: fromNano(w.UpdatedAt),
		ExpiresAt: fromNano(w.ExpiresAt),
	}
}

// kvTTL resolves a session's absolute expiry to a kvstore TTL: zero
// ExpiresAt means no expiry. Remaining lifetimes at or below zero are
// floored against clock skew, mirroring the redis adapter.
func kvTTL(expiresAt time.Time, now time.Time) time.Duration {
	if expiresAt.IsZero() {
		return 0
	}

	return max(expiresAt.Sub(now), DefaultMinPreserveTTL)
}

// Create mints a fresh session with the given ttl (<= 0 means the store
// default). Expiry is enforced by the kv row at write time.
func (d *driver) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	if err := ctx.Err(); err != nil {
		return session.Session{}, err
	}

	if d.closed.Load() {
		return session.Session{}, session.ErrClosed
	}

	if ttl <= 0 {
		ttl = d.ttl
	}

	sess := session.NewSession(session.NewID(), ttl)

	// wireSession is {map, int64 x3}: encoding/json cannot fail on it
	// (nil maps, integers), so the error is provably infallible and
	// discarded.
	buf, _ := sessionCodec.Encode(toWire(sess))

	if err := d.kv.Set(ctx, d.key(sess.ID), buf, kvTTL(sess.ExpiresAt, time.Now())); err != nil {
		return session.Session{}, fmt.Errorf("db: create set: %w", err)
	}

	return sess.Clone(), nil
}

// Get returns the session for id, or ErrNotFound on miss or expiry
// (expired sessions are indistinguishable from missing ones; the expired
// row is deleted best-effort). A corrupt record fails closed with a
// wrapped error and is never replayed.
func (d *driver) Get(ctx context.Context, id string) (session.Session, error) {
	if err := ctx.Err(); err != nil {
		return session.Session{}, err
	}

	if err := session.ValidateID(id); err != nil {
		return session.Session{}, fmt.Errorf("db: %w", err)
	}

	if d.closed.Load() {
		return session.Session{}, session.ErrClosed
	}

	raw, ok, err := d.kv.Get(ctx, d.key(id))
	if err != nil {
		return session.Session{}, fmt.Errorf("db: get: %w", err)
	}

	if !ok {
		return session.Session{}, session.ErrNotFound
	}

	w, err := sessionCodec.Decode(raw)
	if err != nil {
		_ = d.kv.Delete(ctx, d.key(id))

		return session.Session{}, fmt.Errorf("db: decode: %w", err)
	}

	sess := fromWire(id, w)

	if sess.Expired(time.Now()) {
		_ = d.kv.Delete(ctx, d.key(id))

		return session.Session{}, session.ErrNotFound
	}

	return sess, nil
}

// Save upserts sess, touches UpdatedAt, and preserves the original
// absolute ExpiresAt for live records (never extends it). A missing,
// expired, or corrupt record is written fresh under the store default TTL,
// ignoring any caller-provided ExpiresAt.
func (d *driver) Save(ctx context.Context, sess session.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := session.ValidateID(sess.ID); err != nil {
		return fmt.Errorf("db: %w", err)
	}

	if d.closed.Load() {
		return session.ErrClosed
	}

	now := time.Now()
	createdAt := now
	expiresAt := now.Add(d.ttl)

	raw, ok, err := d.kv.Get(ctx, d.key(sess.ID))
	if err != nil {
		return fmt.Errorf("db: save get: %w", err)
	}

	if ok {
		if existing, derr := sessionCodec.Decode(raw); derr == nil {
			cur := fromWire(sess.ID, existing)
			if !cur.Expired(now) {
				createdAt = cur.CreatedAt
				expiresAt = cur.ExpiresAt
			}
		}
		// Expired or corrupt: fall through to the fresh defaults above.
	}

	buf, err := sessionCodec.Encode(wireSession{
		Data:      sess.Data,
		CreatedAt: unixNano(createdAt),
		UpdatedAt: unixNano(now),
		ExpiresAt: unixNano(expiresAt),
	})
	if err != nil {
		return fmt.Errorf("db: encode: %w", err)
	}

	if err := d.kv.Set(ctx, d.key(sess.ID), buf, kvTTL(expiresAt, now)); err != nil {
		return fmt.Errorf("db: save set: %w", err)
	}

	return nil
}

// Delete removes id; missing IDs return nil.
func (d *driver) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := session.ValidateID(id); err != nil {
		return fmt.Errorf("db: %w", err)
	}

	if d.closed.Load() {
		return session.ErrClosed
	}

	if err := d.kv.Delete(ctx, d.key(id)); err != nil {
		return fmt.Errorf("db: delete: %w", err)
	}

	return nil
}

// Close marks the store closed; it is idempotent. Drivers built via New
// also close their owned connection; drivers built via NewFromDB borrow
// the caller's DB and leave it open.
func (d *driver) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if !d.owns {
		return nil
	}

	return d.db.Close(context.Background())
}
