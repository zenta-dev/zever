// Package db provides a durable outbox.Store backed by sqlite or postgres.
//
// Record inserts the message in the caller's transaction, so the event and
// the business row commit together. A polling relay claims a batch with an
// atomic UPDATE ... RETURNING (postgres adds FOR UPDATE SKIP LOCKED, sqlite
// serializes on a process-local mutex), publishes each message through the
// configured outbox.Publisher, marks successes processed, and retries
// failures with shared/retry backoff. Messages that exhaust MaxAttempts are
// marked failed and left for inspection. Processed rows older than Retention
// are deleted. The adapter also implements outbox.Inbox: Process records an
// event ID in the caller's transaction and runs the side effect only once.
//
// Trace context captured at Record time is injected into the stored headers
// and delivered on publish, so the consumer can continue the originating
// trace.
//
// Pool sharing: the container resolves one pool per DSN and shares it across
// batteries pointing at the same DSN; OpenFromDB/RegisterShared borrow it.
// Set dedicated_pool: true for a private pool.
package db
