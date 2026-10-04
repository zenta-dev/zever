package kvstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// DefaultTable is the key-value table created when New is given an empty
// table name.
const DefaultTable = "kv_store"

// DefaultPingTimeout bounds the connectivity check during construction.
const DefaultPingTimeout = 3 * time.Second

// MaxKeyLen caps key length in bytes; larger keys fail Set/Get/Delete.
const MaxKeyLen = 1024

// ErrInvalidKey is returned when a key is empty or exceeds MaxKeyLen.
var ErrInvalidKey = errors.New("kvstore: invalid key")

// ErrInvalidTable is returned when the table name is outside the
// identifier shape the DDL interpolation requires.
var ErrInvalidTable = errors.New("kvstore: invalid table")

// ErrInvalidInteger is returned when AddDelta finds a present, unexpired
// record whose value is not a base-10 int64. Future cache/db adapters map
// it to cache.InvalidValueError{Key}.
var ErrInvalidInteger = errors.New("kvstore: invalid integer")

// ErrUnsupportedType is returned when coerceValue or coerceTime encounters
// a Go type it cannot convert.
var ErrUnsupportedType = errors.New("unsupported type")

// ErrLostUpdate is returned when AddDelta's UPDATE affects zero rows,
// indicating a concurrent modification.
var ErrLostUpdate = errors.New("lost update")

// kvRow is the key-value entity. Column order matches kvColumns: the
// positional Scan must read them in exactly this order.
type kvRow struct {
	Key       string
	Value     []byte
	ExpiresAt time.Time
}

// kvColumns is the entity column list in Scan order.
var kvColumns = []string{"key", "value", "expires_at"}

// Scan reads one row positionally, coercing driver representations:
// timestamps arrive as time.Time (postgres) or RFC3339Nano text (sqlite),
// values as []byte (both) or string (sqlite text affinity).
func (r *kvRow) Scan(row orm.Row) error {
	var key string
	var valueRaw, expiresRaw any

	if err := row.Scan(&key, &valueRaw, &expiresRaw); err != nil {
		return err
	}

	value, err := coerceValue(valueRaw)
	if err != nil {
		return fmt.Errorf("kvstore: scan value: %w", err)
	}

	expires, err := coerceTime(expiresRaw)
	if err != nil {
		return fmt.Errorf("kvstore: scan expires_at: %w", err)
	}

	*r = kvRow{Key: key, Value: value, ExpiresAt: expires}

	return nil
}

// coerceValue converts a value cell to []byte.
func coerceValue(v any) ([]byte, error) {
	switch b := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return b, nil
	case string:
		return []byte(b), nil
	default:
		return nil, fmt.Errorf("kvstore: unsupported value %T: %w", v, ErrUnsupportedType)
	}
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
		return time.Time{}, fmt.Errorf("kvstore: unsupported timestamp %T: %w", v, ErrUnsupportedType)
	}
}

// Store is a generic key-value store with TTL over a coredb.DB. It borrows
// its DB and never closes it. It is safe for concurrent use.
type Store struct {
	db        coredb.DB
	tbl       orm.Table[kvRow]
	cKey      orm.Column[kvRow, string]
	cValue    orm.Column[kvRow, []byte]
	cExpires  orm.Column[kvRow, time.Time]
	tableName string
}

// New creates a Store over db, creating the key-value table when missing.
// An empty table selects DefaultTable. Only sqlite and postgres dialects
// are supported; anything else fails closed with
// dialect.ErrUnsupportedByDialect. The caller retains ownership of db.
func New(db coredb.DB, table string) (*Store, error) {
	if db == nil {
		return nil, errors.New("kvstore: db must not be nil")
	}

	if table == "" {
		table = DefaultTable
	}

	if err := validateTableName(table); err != nil {
		return nil, err
	}

	s := &Store{
		db:        db,
		tbl:       orm.NewTable[kvRow](table, kvColumns),
		cKey:      orm.NewColumn[kvRow, string](table, "key"),
		cValue:    orm.NewColumn[kvRow, []byte](table, "value"),
		cExpires:  orm.NewColumn[kvRow, time.Time](table, "expires_at"),
		tableName: table,
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("kvstore: ping: %w", err)
	}

	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

// checkDialect fails closed on dialects outside sqlite/postgres.
func (s *Store) checkDialect() error {
	switch s.db.Dialect() {
	case "sqlite", "postgres":
		return nil
	default:
		return fmt.Errorf("kvstore: unsupported dialect %q: %w",
			s.db.Dialect(), dialect.ErrUnsupportedByDialect)
	}
}

// ensureSchema creates the key-value table when missing. DDL only: every
// state transition below goes through the orm typed builder.
func (s *Store) ensureSchema(ctx context.Context) error {
	if err := s.checkDialect(); err != nil {
		return err
	}

	quoted := `"` + strings.ReplaceAll(s.tableName, `"`, `""`) + `"`

	var ddl string

	if s.db.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`key TEXT PRIMARY KEY, ` +
			`value BYTEA NOT NULL, ` +
			`expires_at TIMESTAMPTZ NOT NULL)`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS ` + quoted + ` (` +
			`key TEXT PRIMARY KEY, ` +
			`value BLOB NOT NULL, ` +
			`expires_at TEXT NOT NULL)`
	}

	if _, err := s.db.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("kvstore: ensure schema: %w", err)
	}

	return nil
}

// validateTableName rejects table names outside [A-Za-z_][A-Za-z0-9_]*
// because the name is interpolated into DDL (orm has no DDL builder).
func validateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: table must not be empty", ErrInvalidTable)
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')

		if !ok {
			return fmt.Errorf("%w: %q", ErrInvalidTable, name)
		}
	}

	return nil
}

// validateKey rejects empty keys and keys longer than MaxKeyLen.
func validateKey(key string) error {
	if key == "" || len(key) > MaxKeyLen {
		return fmt.Errorf("%w: length %d", ErrInvalidKey, len(key))
	}

	return nil
}

// expiredAt resolves ttl to an absolute expiry: ttl <= 0 means no expiry
// (zero time, which never expires).
func expiredAt(now time.Time, ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}

	return now.Add(ttl)
}

// expired reports whether a record expiring at exp is expired at now. A
// zero exp never expires; expiry itself counts as expired.
func expired(exp, now time.Time) bool {
	if exp.IsZero() {
		return false
	}

	return !now.Before(exp)
}

// Get returns the value for key. Misses and expired records report
// ok=false with a nil error; expired records are deleted best-effort so a
// later SweepExpired never sees them. The returned slice is a copy owned
// by the caller.
func (s *Store) Get(ctx context.Context, key string) (value []byte, ok bool, err error) {
	value, _, ok, err = s.GetWithExpiry(ctx, key)

	return value, ok, err
}

// GetWithExpiry is Get plus the record's absolute expiry: zero means the
// record never expires. Callers use it to preserve a record's original
// deadline across overwrites (e.g. completing a reservation without
// extending it).
func (s *Store) GetWithExpiry(ctx context.Context, key string) (value []byte, expiresAt time.Time, ok bool, err error) {
	if cerr := ctx.Err(); cerr != nil {
		return nil, time.Time{}, false, cerr
	}

	if verr := validateKey(key); verr != nil {
		return nil, time.Time{}, false, verr
	}

	if derr := s.checkDialect(); derr != nil {
		return nil, time.Time{}, false, derr
	}

	row, found, qerr := orm.From[kvRow, *kvRow](s.tbl).Where(s.cKey.Eq(key)).First(ctx, s.db)
	if qerr != nil {
		return nil, time.Time{}, false, fmt.Errorf("kvstore: get %q: %w", key, qerr)
	}

	if !found {
		return nil, time.Time{}, false, nil
	}

	if expired(row.ExpiresAt, time.Now()) {
		_, _ = orm.DeleteFrom(s.tbl).Where(s.cKey.Eq(key)).Exec(ctx, s.db)

		return nil, time.Time{}, false, nil
	}

	return append([]byte(nil), row.Value...), row.ExpiresAt, true, nil
}

// Set upserts value under key with a TTL: ttl <= 0 means no expiry. The
// write is atomic per row (single upsert statement); the stored slice is
// copied so later caller mutations never affect stored state.
func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateKey(key); err != nil {
		return err
	}

	if err := s.checkDialect(); err != nil {
		return err
	}

	cp := append([]byte(nil), value...)
	exp := expiredAt(time.Now(), ttl)

	if err := orm.InsertInto(s.tbl).Values(
		orm.Set(s.cKey, key),
		orm.Set(s.cValue, cp),
		orm.Set(s.cExpires, exp),
	).OnConflict(s.cKey.Col()).DoUpdate(
		orm.Set(s.cValue, cp),
		orm.Set(s.cExpires, exp),
	).Exec(ctx, s.db); err != nil {
		return fmt.Errorf("kvstore: set %q: %w", key, err)
	}

	return nil
}

// SetNX inserts value under key with a TTL, reporting whether the insert
// won: a present (even expired — expiry is lazy) key reports won=false
// with a nil error instead of failing. Callers that treat expiry as
// absence should Get (which purges expired rows) and retry on won=false,
// mirroring the claim/retry shape of the idempotency redis adapter.
// The input slice is copied; ttl <= 0 means no expiry.
func (s *Store) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (won bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if err := validateKey(key); err != nil {
		return false, err
	}

	if err := s.checkDialect(); err != nil {
		return false, err
	}

	cp := append([]byte(nil), value...)
	exp := expiredAt(time.Now(), ttl)

	if err := orm.InsertInto(s.tbl).Values(
		orm.Set(s.cKey, key),
		orm.Set(s.cValue, cp),
		orm.Set(s.cExpires, exp),
	).Exec(ctx, s.db); err != nil {
		if isDuplicateErr(err) {
			return false, nil
		}

		return false, fmt.Errorf("kvstore: setnx %q: %w", key, err)
	}

	return true, nil
}

// AddDelta atomically adds delta to the decimal integer stored under key
// and returns the new value. A missing or expired record behaves as base 0
// (expired rows are deleted and recreated with no expiry, mirroring Get's
// lazy delete); a present record keeps its existing expires_at — only the
// value column is rewritten. Values are stored as base-10 ASCII bytes, so a
// present but non-numeric value fails with ErrInvalidInteger and the stored
// value is left untouched.
//
// Atomicity is per-row read-modify-write inside a transaction (SELECT, then
// ParseInt in Go, then UPDATE or INSERT): Go-side parsing is what keeps
// sqlite honest, where CAST(value AS INTEGER) would silently coerce "abc" to
// 0. Concurrent increments on the same key are serialized by the database
// write lock; under postgres ReadCommitted two overlapping transactions can
// still interleave their reads (no SELECT FOR UPDATE — sqlite rejects it),
// so strict counter use under high contention should retry on conflict
// rather than assume single-statement linearity.
func (s *Store) AddDelta(ctx context.Context, key string, delta int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if err := validateKey(key); err != nil {
		return 0, err
	}

	if err := s.checkDialect(); err != nil {
		return 0, err
	}

	var out int64

	err := orm.WithNestedTx(ctx, s.db, func(txCtx context.Context, tx coredb.Tx) error {
		row, found, qerr := orm.From[kvRow, *kvRow](s.tbl).Where(s.cKey.Eq(key)).First(txCtx, tx)
		if qerr != nil {
			return fmt.Errorf("kvstore: adddelta %q: %w", key, qerr)
		}

		var (
			cur    int64
			exists bool
		)

		if found {
			if expired(row.ExpiresAt, time.Now()) {
				if _, derr := orm.DeleteFrom(s.tbl).Where(s.cKey.Eq(key)).Exec(txCtx, tx); derr != nil {
					return fmt.Errorf("kvstore: adddelta %q: %w", key, derr)
				}
			} else {
				n, perr := strconv.ParseInt(string(row.Value), 10, 64)
				if perr != nil {
					return fmt.Errorf("kvstore: adddelta %q: %w: %w", key, ErrInvalidInteger, perr)
				}

				cur = n
				exists = true
			}
		}

		out = cur + delta
		raw := []byte(strconv.FormatInt(out, 10))

		if !exists {
			if ierr := orm.InsertInto(s.tbl).Values(
				orm.Set(s.cKey, key),
				orm.Set(s.cValue, raw),
				orm.Set(s.cExpires, time.Time{}),
			).Exec(txCtx, tx); ierr != nil {
				return fmt.Errorf("kvstore: adddelta %q: %w", key, ierr)
			}

			return nil
		}

		n, uerr := orm.UpdateTable(s.tbl).Where(s.cKey.Eq(key)).
			Set(orm.Set(s.cValue, raw)).Exec(txCtx, tx)
		if uerr != nil {
			return fmt.Errorf("kvstore: adddelta %q: %w", key, uerr)
		}

		if n != 1 {
			return fmt.Errorf("kvstore: adddelta %q: %w", key, ErrLostUpdate)
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return out, nil
}

// Delete removes key; missing keys return nil.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateKey(key); err != nil {
		return err
	}

	if err := s.checkDialect(); err != nil {
		return err
	}

	if _, err := orm.DeleteFrom(s.tbl).Where(s.cKey.Eq(key)).Exec(ctx, s.db); err != nil {
		return fmt.Errorf("kvstore: delete %q: %w", key, err)
	}

	return nil
}

// SweepExpired deletes all expired records and returns the number removed.
// Records with no expiry (zero expires_at) are never swept.
func (s *Store) SweepExpired(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if err := s.checkDialect(); err != nil {
		return 0, err
	}

	now := time.Now()

	n, err := orm.DeleteFrom(s.tbl).Where(
		orm.And(s.cExpires.Neq(time.Time{}), s.cExpires.Lte(now)),
	).Exec(ctx, s.db)
	if err != nil {
		return 0, fmt.Errorf("kvstore: sweep: %w", err)
	}

	return n, nil
}

// isDuplicateErr reports PRIMARY KEY / UNIQUE violations across dialects.
func isDuplicateErr(err error) bool {
	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "primary key")
}
