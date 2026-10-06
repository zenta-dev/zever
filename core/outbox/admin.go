package outbox

import (
	"context"
	"time"
)

// Admin is the outbox operational surface: inspect and repair the
// dead-letter queue without starting the relay. It is deliberately separate
// from Store so that adding DLQ operations never breaks existing Store
// implementers.
//
// Implementations are expected to be safe for concurrent use with a running
// relay.
type Admin interface {
	// List returns up to limit messages with the given status, oldest first.
	// An empty status defaults to "failed" (the DLQ). A non-positive limit
	// applies the adapter's default page size.
	List(ctx context.Context, status string, limit int) ([]Message, error)
	// Requeue moves failed messages back to pending, resetting their attempt
	// count and lock. An empty id requeues every failed message.
	Requeue(ctx context.Context, id string) error
	// Purge deletes processed messages older than before and returns the
	// number of rows removed.
	Purge(ctx context.Context, before time.Time) (int64, error)
}

// Deleter is the optional per-message deletion capability an outbox adapter
// may expose for DLQ operations (for example `zever outbox dlq purge --id`).
// It is separate from Admin so an adapter can offer inspection/requeue without
// committing to destructive deletes.
type Deleter interface {
	// Delete removes the message with the given id, returning ErrNotFound
	// when no such message exists.
	Delete(ctx context.Context, id string) error
}
