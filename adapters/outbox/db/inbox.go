package db

import (
	"context"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// Process records eventID in tx and, when it is new, runs fn in that same
// transaction. A repeated eventID returns nil without running fn, making
// consumers idempotent under at-least-once delivery. A nil tx fails with
// ErrTxRequired.
func (d *driver) Process(ctx context.Context, tx coredb.Tx, eventID string, fn func(context.Context, coredb.Tx) error) error {
	if tx == nil {
		return fmt.Errorf("db: process: %w", outbox.ErrTxRequired)
	}

	if eventID == "" {
		return outbox.InvalidMessageError{Reason: "event id must be non-empty"}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	n, err := tx.Exec(ctx,
		`INSERT INTO `+quoteIdent(d.inboxTable)+` (event_id, processed_at) VALUES (?, ?) ON CONFLICT (event_id) DO NOTHING`,
		eventID, d.ts(time.Now().UTC()),
	)
	if err != nil {
		return fmt.Errorf("db: process %q: %w", eventID, err)
	}

	if n == 0 {
		return nil
	}

	return fn(ctx, tx)
}
