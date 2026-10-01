// Package kvstore provides a shared generic key-value store with TTL over
// coredb.DB, so DB-backed adapters stop hand-rolling their own expiry
// tables.
//
// One table holds key TEXT PRIMARY KEY, value BLOB, and expires_at
// (TIMESTAMPTZ on postgres, RFC3339Nano text on sqlite). Every state
// transition goes through the orm typed builder; only schema creation uses
// raw DDL (orm has no DDL builder).
//
// Expiry is absolute per record: a ttl <= 0 on Set means no expiry.
// Expired records read as missing (Get returns ok=false) and are removed
// lazily on read plus by an explicit SweepExpired call; there is no
// background goroutine.
//
// The store borrows its coredb.DB and never closes it. It is safe for
// concurrent use.
package kvstore
