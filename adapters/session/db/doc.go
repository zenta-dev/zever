// Package db provides a DB-backed session.Store over shared/kvstore.
//
// Sessions are stored as a single JSON value per key (prefix + ":" + id)
// with an absolute expiry enforced by the kv row's expires_at: like the
// redis adapter's SET EX, no access is needed for a session to expire.
// Expired rows read as missing and are purged lazily on read plus by
// kvstore.SweepExpired.
//
// Save preserves the original absolute ExpiresAt with a read-then-write:
// a concurrent change to the same key between the read and the write may
// win (last-write-wins on Data), unlike the redis adapter's
// WATCH/MULTI/EXEC pair. Corrupt records fail closed and are never
// replayed; Save overwrites them with a fresh expiry.
package db
