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
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/shared/kvstore"
)

// Compile-time check that driver implements cache.Cache.
var _ cache.Cache = (*driver)(nil)

// driver is a DB-backed cache.Cache. It is safe for concurrent use.
type driver struct {
	db     coredb.DB
	kv     *kvstore.Store
	prefix string
	owns   bool
	closed atomic.Bool
}

// New creates a DB-backed cache.Cache. Empty DSN selects sqlite at Path
// (default ":memory:"); a set DSN opens postgres. The cache table is
// created when missing. The driver owns its connection: Close releases it.
func New(o Options) (cache.Cache, error) {
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

// NewFromDB creates a DB-backed cache.Cache over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned Cache does not close db, and a failed
// NewFromDB never closes db.
func NewFromDB(conn coredb.DB, o Options) (cache.Cache, error) {
	if conn == nil {
		return nil, errors.New("db: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed cache.Cache over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (cache.Cache, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/prefix defaults, opens the kv store, and wires
// the driver. owns reports whether the driver owns conn and may close it
// in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (cache.Cache, error) {
	table := o.Table
	if table == "" {
		table = DefaultTable
	}

	prefix := o.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}

	kv, err := kvstore.New(conn, table)
	if err != nil {
		return nil, err
	}

	return &driver{db: conn, kv: kv, prefix: prefix, owns: owns}, nil
}

// key namespaces a cache key under the store prefix.
func (d *driver) key(key string) string {
	return d.prefix + ":" + key
}

// Get returns the value stored under key, or NotFoundError on miss or
// expiry (expired entries read as missing; the row is purged lazily).
func (d *driver) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if d.closed.Load() {
		return nil, cache.ErrClosed
	}

	raw, ok, err := d.kv.Get(ctx, d.key(key))
	if err != nil {
		return nil, fmt.Errorf("db: get: %w", err)
	}

	if !ok {
		return nil, &cache.NotFoundError{Key: key}
	}

	return raw, nil
}

// Set stores value under key, overwriting any existing entry. A ttl <= 0
// means persist (no expiry).
func (d *driver) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return cache.ErrClosed
	}

	if err := d.kv.Set(ctx, d.key(key), value, ttl); err != nil {
		return fmt.Errorf("db: set: %w", err)
	}

	return nil
}

// SetIfAbsent stores value under key only when no live entry exists,
// reporting whether the insert won. An expired entry reads as absent: the
// preceding Get purges it, so the claim retries against a clean row,
// mirroring the idempotency redis adapter's claim/retry shape.
func (d *driver) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if d.closed.Load() {
		return false, cache.ErrClosed
	}

	if _, ok, err := d.kv.Get(ctx, d.key(key)); err != nil {
		return false, fmt.Errorf("db: set if absent get: %w", err)
	} else if ok {
		return false, nil
	}

	won, err := d.kv.SetNX(ctx, d.key(key), value, ttl)
	if err != nil {
		return false, fmt.Errorf("db: set if absent: %w", err)
	}

	return won, nil
}

// Delete removes the entry stored under key; missing keys return nil.
func (d *driver) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return cache.ErrClosed
	}

	if err := d.kv.Delete(ctx, d.key(key)); err != nil {
		return fmt.Errorf("db: delete: %w", err)
	}

	return nil
}

// Increment atomically increments the integer value stored under key, basing
// a missing or expired entry at 0. A present but non-integer value fails
// with InvalidValueError and is left untouched.
func (d *driver) Increment(ctx context.Context, key string) error {
	return d.addDelta(ctx, key, 1, "increment")
}

// Decrement atomically decrements the integer value stored under key, basing
// a missing or expired entry at 0. A present but non-integer value fails
// with InvalidValueError and is left untouched.
func (d *driver) Decrement(ctx context.Context, key string) error {
	return d.addDelta(ctx, key, -1, "decrement")
}

// addDelta applies a signed counter step through kvstore.AddDelta, mapping
// kvstore.ErrInvalidInteger to cache.InvalidValueError. The returned total
// is ignorable: the cache contract reports counters via Get.
func (d *driver) addDelta(ctx context.Context, key string, delta int64, op string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if d.closed.Load() {
		return cache.ErrClosed
	}

	if _, err := d.kv.AddDelta(ctx, d.key(key), delta); err != nil {
		if errors.Is(err, kvstore.ErrInvalidInteger) {
			return &cache.InvalidValueError{Key: key, Err: err}
		}

		return fmt.Errorf("db: %s: %w", op, err)
	}

	return nil
}

// Exists reports whether a live entry exists under key.
func (d *driver) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if d.closed.Load() {
		return false, cache.ErrClosed
	}

	_, ok, err := d.kv.Get(ctx, d.key(key))
	if err != nil {
		return false, fmt.Errorf("db: exists: %w", err)
	}

	return ok, nil
}

// Close marks the cache closed; it is idempotent. Drivers built via New
// also close their owned connection; drivers built via NewFromDB borrow the
// caller's DB and leave it open.
func (d *driver) Close(ctx context.Context) error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}

	if !d.owns {
		return nil
	}

	return d.db.Close(ctx)
}
