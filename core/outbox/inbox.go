package outbox

import (
	"context"

	"github.com/zenta-dev/zever/core/db"
)

// Inbox makes consumers idempotent. Process records eventID and runs fn in
// the same transaction: a repeated eventID is a no-op returning nil, so fn's
// side effects apply exactly once even under at-least-once delivery.
type Inbox interface {
	// Process records eventID in tx and, when it is new, runs fn in that same
	// tx. A repeated eventID returns nil without running fn. A nil tx fails
	// with ErrTxRequired.
	Process(ctx context.Context, tx db.Tx, eventID string, fn func(context.Context, db.Tx) error) error
}
