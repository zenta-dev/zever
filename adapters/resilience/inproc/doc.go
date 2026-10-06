// Package inproc provides the in-process resilience adapter.
//
// It composes a circuit breaker (sony/gobreaker/v2), a bulkhead
// (golang.org/x/sync/semaphore), shared/retry backoff, and an overall timeout
// into named per-dependency guards. All state is process-local; the adapter
// requires no external services and is safe for concurrent use.
package inproc
