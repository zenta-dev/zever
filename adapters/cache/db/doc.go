// Package db provides a DB-backed cache.Cache over shared/kvstore.
//
// Entries are stored as raw byte values per key (prefix + ":" + key) with
// the TTL enforced by the kv row's expires_at: like the redis adapter's SET
// EX, no access is needed for an entry to expire. Expired rows read as
// missing and are purged lazily on read plus by kvstore.SweepExpired. A
// per-call ttl <= 0 means persist (no expiry), mirroring the memory and
// redis adapters.
//
// Counters go through kvstore.AddDelta, which is a per-row read-modify-write
// inside a transaction: concurrent increments on the same key serialize on
// the database write lock, but under postgres ReadCommitted two overlapping
// transactions can still interleave their reads (no SELECT FOR UPDATE —
// sqlite rejects it), so the counter is documented non-linearizable under
// contention. Non-integer values fail increments with
// cache.ErrInvalidValue, leaving the stored value untouched.
package db
