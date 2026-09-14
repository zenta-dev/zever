// Package redis provides a Redis-backed idempotency.Store, safe to share
// across processes.
//
// A reservation is a single SET NX with a TTL, which is atomic in Redis,
// so exactly one caller across the whole fleet wins Begin for a given key.
// The stored value is a wire record: a 1-byte tag ('P' pending, 'D' done),
// a 2-byte big-endian fingerprint length, the fingerprint, then (done
// records only) the result. The fingerprint rides alongside so replays and
// completions can scope the key to a request payload: a mismatch reports
// idempotency.ErrKeyMismatch even mid-flight.
package redis
