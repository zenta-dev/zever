package memory

import (
	"context"
	"fmt"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// Options configures the in-memory adapter. It embeds core outbox.Options and
// adds the Publisher the adapter publishes through, since core Options carries
// only the informational string selector.
type Options struct {
	// Options holds the shared outbox configuration.
	outbox.Options
	// Publisher receives every recorded message immediately. Required for
	// Record to succeed.
	Publisher outbox.Publisher
}

// driver is a non-durable outbox.Store. It is safe for concurrent use.
type driver struct {
	publisher outbox.Publisher
}

var _ outbox.Store = (*driver)(nil)

// New creates an in-memory outbox store. A nil Publisher is accepted at
// construction; Record then fails until one is wired.
func New(o Options) (outbox.Store, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	return &driver{publisher: o.Publisher}, nil
}

// Record validates msg and publishes it immediately. It ignores tx: the
// in-memory adapter has no transaction to join.
func (d *driver) Record(ctx context.Context, _ coredb.Tx, msg outbox.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if d.publisher == nil {
		return fmt.Errorf("memory: publisher is nil: %w", outbox.ErrInvalidOptions)
	}

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	return d.publisher.Publish(ctx, msg)
}

// Start is a no-op: Record already published synchronously.
func (d *driver) Start(context.Context) error { return nil }

// Status returns a zero Status: the adapter keeps no counters.
func (d *driver) Status() outbox.Status { return outbox.Status{} }

// Close is a no-op.
func (d *driver) Close() error { return nil }

// Name returns the adapter name.
func (d *driver) Name() string { return string(outbox.Memory) }
