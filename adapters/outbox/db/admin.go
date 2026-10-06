package db

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

// DefaultListLimit caps Admin.List when the caller passes a non-positive
// limit.
const DefaultListLimit = 100

// Outbox message statuses as stored in the status column.
const (
	statusPending   = "pending"
	statusProcessed = "processed"
	statusFailed    = "failed"
)

var (
	_ outbox.Admin   = (*driver)(nil)
	_ outbox.Deleter = (*driver)(nil)
)

// List returns up to limit messages with the given status, oldest first. An
// empty status defaults to "failed" (the DLQ); a non-positive limit applies
// DefaultListLimit.
func (d *driver) List(ctx context.Context, status string, limit int) ([]outbox.Message, error) {
	if status == "" {
		status = statusFailed
	}

	if limit <= 0 {
		limit = DefaultListLimit
	}

	rows, err := d.conn.Query(ctx,
		`SELECT id, topic, "key", payload, headers, created_at, attempts FROM `+
			quoteIdent(d.table)+` WHERE status = ? ORDER BY created_at LIMIT ?`,
		status, limit)
	if err != nil {
		return nil, fmt.Errorf("db: list: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var msgs []outbox.Message

	for rows.Next() {
		c, err := scanClaimed(rows)
		if err != nil {
			return nil, fmt.Errorf("db: list scan: %w", err)
		}

		msgs = append(msgs, outbox.Message{
			ID:        c.ID,
			Topic:     c.Topic,
			Key:       c.Key,
			Payload:   c.Payload,
			Headers:   c.Headers,
			CreatedAt: c.CreatedAt,
			Attempts:  c.Attempts,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: list: %w", err)
	}

	return msgs, nil
}

// Requeue moves failed messages back to pending, clearing attempts, lock, and
// last error. An empty id requeues every failed message.
func (d *driver) Requeue(ctx context.Context, id string) error {
	query := `UPDATE ` + quoteIdent(d.table) +
		` SET status = ?, attempts = 0, locked_until = NULL, last_error = NULL WHERE status = ?`

	args := []any{statusPending, statusFailed}
	if id != "" {
		query += ` AND id = ?`
		args = append(args, id)
	}

	if _, err := d.conn.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("db: requeue: %w", err)
	}

	return nil
}

// Purge deletes processed messages older than before and returns the number
// of rows removed.
func (d *driver) Purge(ctx context.Context, before time.Time) (int64, error) {
	n, err := d.conn.Exec(ctx,
		`DELETE FROM `+quoteIdent(d.table)+` WHERE status = ? AND processed_at < ?`,
		statusProcessed, d.ts(before))
	if err != nil {
		return 0, fmt.Errorf("db: purge: %w", err)
	}

	return n, nil
}

// Delete removes the message with the given id, returning ErrNotFound when no
// such message exists.
func (d *driver) Delete(ctx context.Context, id string) error {
	n, err := d.conn.Exec(ctx, `DELETE FROM `+quoteIdent(d.table)+` WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("db: delete %q: %w", id, err)
	}

	if n == 0 {
		return fmt.Errorf("db: delete %q: %w", id, outbox.ErrNotFound)
	}

	return nil
}
