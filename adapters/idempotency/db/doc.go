// Package db provides a DB-backed idempotency.Store over shared/kvstore.
//
// Reservations are single kv rows holding a binary wire record (pending
// tag or done tag plus fingerprint plus result). Begin claims a key with
// an atomic insert: a lost claim reads the winner back, so concurrent
// beginners serialize on the row exactly like the redis adapter's SET NX.
// A reservation that expired between the failed claim and the follow-up
// read is purged lazily and the claim retried, mirroring the redis
// adapter's beginAttempts loop.
//
// Completed records keep their original absolute expiry (the deadline Begin
// set is never extended by Complete). Corrupt records fail closed and are
// never replayed.
package db
