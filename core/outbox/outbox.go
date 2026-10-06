package outbox

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/shared/registry"
)

// Store is the outbox backend.
//
// Record persists msg inside tx, the same transaction as the business write;
// a nil tx fails with ErrTxRequired. Start launches the relay that publishes
// recorded messages; it is idempotent. Status reports relay counters. Close
// stops the relay and releases resources; it is idempotent.
type Store interface {
	// Record persists msg in tx. A nil tx fails with ErrTxRequired.
	Record(ctx context.Context, tx db.Tx, msg Message) error
	// Start launches the relay loop. It is idempotent.
	Start(ctx context.Context) error
	// Status reports current relay counters.
	Status() Status
	// Close stops the relay and releases resources. It is idempotent.
	Close() error
	// Name returns the adapter name.
	Name() string
}

// Publisher delivers a recorded message to its destination transport. The db
// adapter bridges core/eventbus or core/queue to this interface in
// application wiring, keeping core/outbox transport-agnostic.
type Publisher interface {
	// Publish delivers msg. A non-nil error leaves the message pending for
	// retry (or marks it failed once MaxAttempts is reached).
	Publish(ctx context.Context, msg Message) error
}

// PublisherFunc adapts a plain function to Publisher.
type PublisherFunc func(ctx context.Context, msg Message) error

// Publish calls f.
func (f PublisherFunc) Publish(ctx context.Context, msg Message) error {
	return f(ctx, msg)
}

// Status reports relay counters and health.
type Status struct {
	// Pending is the number of messages awaiting publication.
	Pending int64
	// Processed is the number of successfully published messages.
	Processed int64
	// Failed is the number of messages that exhausted MaxAttempts.
	Failed int64
	// LastError is the most recent relay or publish error, if any.
	LastError string
	// Stalled reports that pending messages are not draining.
	Stalled bool
}

// Factory creates a Store from the given Options.
type Factory func(opts Options) (Store, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return DuplicateAdapterError{Adapter: adapter} },
	func(adapter Adapter) error { return UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Store for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("outbox: open %s: %w", adapter, err)
	}

	return s, nil
}

// Default returns the zero-infra default options: the durable db adapter on a
// private sqlite database with adapter defaults for every tunable.
func Default() Options {
	return Options{
		Table:        DefaultTable,
		InboxTable:   DefaultInboxTable,
		PollInterval: DefaultPollInterval,
		BatchSize:    DefaultBatchSize,
		MaxAttempts:  DefaultMaxAttempts,
		Retention:    DefaultRetention,
		LockSeconds:  DefaultLockSeconds,
	}
}
