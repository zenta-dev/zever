// Package redis provides the Redis resilience adapter.
//
// It composes a distributed circuit breaker (sony/gobreaker/v2 backed by the
// gobreaker redis SharedDataStore), a process-local bulkhead
// (golang.org/x/sync/semaphore), shared/retry backoff, and an overall timeout
// into named per-dependency guards. Only the circuit-breaker state is shared
// through Redis, so independent replicas trip together; timeout, retry, and
// bulkhead stay process-local because their state is not meaningful to share.
// Safe for concurrent use.
package redis
