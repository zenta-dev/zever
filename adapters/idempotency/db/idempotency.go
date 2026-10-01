package db

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/shared/kvstore"
)

const (
	tagPending = 'P'
	tagDone    = 'D'

	// beginAttempts bounds the retry loop for the (rare) case where a
	// reservation expires between our failed claim and the follow-up
	// read.
	beginAttempts = 3
)

// Compile-time check that driver implements idempotency.Store.
var _ idempotency.Store = (*driver)(nil)

// driver is a DB-backed idempotency.Store. It is safe for concurrent use.
type driver struct {
	db     coredb.DB
	kv     *kvstore.Store
	prefix string
	ttl    time.Duration
	owns   bool
	closed atomic.Bool
}

// New creates a DB-backed idempotency.Store. Empty DSN selects sqlite at
// Path (default ":memory:"); a set DSN opens postgres. The idempotency
// table is created when missing. The driver owns its connection: Close
// releases it.
func New(o Options) (idempotency.Store, error) {
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

// NewFromDB creates a DB-backed idempotency.Store over an already-open
// coredb.DB, skipping DSN/Path construction. The caller retains ownership
// of db: Close on the returned Store does not close db, and a failed
// NewFromDB never closes db.
func NewFromDB(conn coredb.DB, o Options) (idempotency.Store, error) {
	if conn == nil {
		return nil, errors.New("db: db must not be nil")
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return openFromDB(conn, o, false)
}

// OpenFromDB creates a DB-backed idempotency.Store over an already-open
// coredb.DB; see NewFromDB.
func OpenFromDB(conn coredb.DB, o Options) (idempotency.Store, error) {
	return NewFromDB(conn, o)
}

// openFromDB resolves table/prefix/ttl defaults, opens the kv store, and
// wires the driver. owns reports whether the driver owns conn and may
// close it in Close.
func openFromDB(conn coredb.DB, o Options, owns bool) (idempotency.Store, error) {
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
		ttl = idempotency.DefaultTTL
	}

	kv, err := kvstore.New(conn, table)
	if err != nil {
		return nil, err
	}

	return &driver{db: conn, kv: kv, prefix: prefix, ttl: ttl, owns: owns}, nil
}

func (d *driver) kvKey(key string) string {
	return d.prefix + key
}

// Begin atomically checks for an existing record and reserves key on miss:
// miss returns ({false, nil}, nil) and the caller must execute, then call
// Complete. A hit on a completed record with matching fingerprint returns
// ({true, copy}, nil). A hit on a pending record returns ErrInProgress.
// Fingerprint is checked FIRST on any existing record. All errors are
// fail-closed.
func (d *driver) Begin(ctx context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Outcome{}, fmt.Errorf("db: begin: %w", err)
	}

	if d.closed.Load() {
		return idempotency.Outcome{}, idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return idempotency.Outcome{}, fmt.Errorf("db: %w", err)
	}

	if err := checkFingerprint(opts.Fingerprint); err != nil {
		return idempotency.Outcome{}, err
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = d.ttl
	}

	k := d.kvKey(key)

	for range beginAttempts {
		won, err := d.kv.SetNX(ctx, k, encodePending(opts.Fingerprint), ttl)
		if err != nil {
			return idempotency.Outcome{}, fmt.Errorf("db: begin claim: %w", err)
		}

		if won {
			return idempotency.Outcome{}, nil
		}

		raw, ok, err := d.kv.Get(ctx, k)
		if err != nil {
			return idempotency.Outcome{}, fmt.Errorf("db: begin get: %w", err)
		}

		if !ok {
			// Winner expired mid-race; re-attempt the claim.
			continue
		}

		tag, stored, result, err := decode(raw)
		if err != nil {
			// Never replay garbage: fail closed.
			return idempotency.Outcome{}, fmt.Errorf("db: begin decode: %w", err)
		}

		if !idempotency.FingerprintMatches(stored, opts.Fingerprint) {
			return idempotency.Outcome{}, fmt.Errorf("db: begin: %w", idempotency.ErrKeyMismatch)
		}

		if tag == tagPending {
			return idempotency.Outcome{}, fmt.Errorf("db: begin: %w", idempotency.ErrInProgress)
		}

		out := idempotency.Outcome{Replay: true}
		if len(result) > 0 {
			out.Result = append([]byte(nil), result...)
		}

		return out, nil
	}

	return idempotency.Outcome{}, fmt.Errorf("db: begin: %w", idempotency.ErrInProgress)
}

// Complete stores result for key and marks the record done. It reads the
// existing record first so a fingerprint mismatch reports ErrKeyMismatch
// without overwriting. A missing record (no prior Begin) is upserted under
// the store TTL. A completed record inherits the remaining TTL via the
// record's absolute expiry; only an expired-missing record uses a fresh
// TTL. A corrupt record fails closed and is never overwritten.
func (d *driver) Complete(ctx context.Context, key string, fingerprint, result []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("db: complete: %w", err)
	}

	if d.closed.Load() {
		return idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("db: %w", err)
	}

	if err := checkFingerprint(fingerprint); err != nil {
		return err
	}

	k := d.kvKey(key)
	done := encodeDone(fingerprint, result)

	raw, expiresAt, ok, err := d.kv.GetWithExpiry(ctx, k)
	if err != nil {
		return fmt.Errorf("db: complete get: %w", err)
	}

	if !ok {
		if serr := d.kv.Set(ctx, k, done, d.ttl); serr != nil {
			return fmt.Errorf("db: complete set: %w", serr)
		}

		return nil
	}

	_, stored, _, err := decode(raw)
	if err != nil {
		return fmt.Errorf("db: complete decode: %w", err)
	}

	if !idempotency.FingerprintMatches(stored, fingerprint) {
		return fmt.Errorf("db: complete: %w", idempotency.ErrKeyMismatch)
	}

	// Overwrite the reservation in place, preserving the deadline Begin
	// set (mirrors the redis adapter's KeepTTL). Only a record with no
	// expiry recorded falls back to a fresh store TTL.
	ttl := d.ttl
	if !expiresAt.IsZero() {
		ttl = max(time.Until(expiresAt), DefaultMinPreserveTTL)
	}

	if serr := d.kv.Set(ctx, k, done, ttl); serr != nil {
		return fmt.Errorf("db: complete set: %w", serr)
	}

	return nil
}

// Forget removes key; missing keys return nil.
func (d *driver) Forget(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("db: forget: %w", err)
	}

	if d.closed.Load() {
		return idempotency.ErrClosed
	}

	if err := idempotency.ValidateKey(key); err != nil {
		return fmt.Errorf("db: %w", err)
	}

	if err := d.kv.Delete(ctx, d.kvKey(key)); err != nil {
		return fmt.Errorf("db: forget: %w", err)
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

// maxFingerprintLen caps per-call fingerprints: hashes are tiny, wire
// length is 2 bytes, and unbounded values would abuse store memory.
const maxFingerprintLen = 4096

func checkFingerprint(fp []byte) error {
	if len(fp) > maxFingerprintLen {
		return fmt.Errorf("db: fingerprint %d bytes exceeds %d: %w", len(fp), maxFingerprintLen, idempotency.ErrFingerprintTooLarge)
	}

	return nil
}

// encodePending builds a pending wire record: tag + fpLen + fingerprint.
func encodePending(fp []byte) []byte {
	if len(fp) > math.MaxInt-3 {
		panic("db: encodePending size overflow")
	}
	size := 3 + len(fp)

	out := make([]byte, 0, size)
	out = append(out, tagPending, byte((len(fp)>>8)&0xff), byte(len(fp)&0xff))

	return append(out, fp...)
}

// encodeDone builds a completed wire record: tag + fpLen + fingerprint +
// result.
func encodeDone(fp, result []byte) []byte {
	if len(fp) > math.MaxInt-3 {
		panic("db: encodeDone size overflow")
	}
	base := 3 + len(fp)
	if len(result) > math.MaxInt-base {
		panic("db: encodeDone size overflow")
	}
	size := base + len(result)

	out := make([]byte, 0, size)
	out = append(out, tagDone, byte((len(fp)>>8)&0xff), byte(len(fp)&0xff))
	out = append(out, fp...)

	return append(out, result...)
}

// decode splits a wire record into tag, fingerprint, and result.
// Result aliases raw; callers needing ownership must copy.
func decode(raw []byte) (tag byte, fp, result []byte, err error) {
	if len(raw) < 3 {
		return 0, nil, nil, fmt.Errorf("%w: record too short: %d bytes", idempotency.ErrCorruptRecord, len(raw))
	}

	tag = raw[0]
	if tag != tagPending && tag != tagDone {
		return 0, nil, nil, fmt.Errorf("%w: unknown tag %q", idempotency.ErrCorruptRecord, tag)
	}

	n := int(binary.BigEndian.Uint16(raw[1:3]))
	if len(raw) < 3+n {
		return 0, nil, nil, fmt.Errorf("%w: fingerprint length %d exceeds record %d bytes", idempotency.ErrCorruptRecord, n, len(raw))
	}

	fp = raw[3 : 3+n]
	result = raw[3+n:]

	return tag, fp, result, nil
}
