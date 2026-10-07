package container

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/outboxbridge"
)

// OutboxRelay resolves the outbox store, attaches a publisher derived from the
// configured transport, and starts the relay. It returns the started store.
//
// The returned store is the same cached instance `Outbox()` resolves and
// `Close` shuts down, so application code can keep using `Outbox()` for Record
// after starting the relay here.
//
// Transport selector: `outbox.publisher` picks the destination —
// PublisherEventBus ("eventbus") publishes through the resolved
// `eventbus.EventBus`, while PublisherQueue ("queue") and the empty default
// publish through the resolved `queue.Queue`. The empty default prefers the
// queue because a queued message survives a subscriber restart, which is the
// safer default for a relay that must not lose business events. Any other value
// is rejected earlier: `outbox.Options.Validate` runs inside `Outbox()` before
// this accessor reads the selector.
//
// The transport failing to resolve is a boot error, never a silent no-op: an
// outbox relay that claims messages and drops them looks healthy right up
// until the events are gone. Callers should treat a non-nil return as a
// startup failure.
//
// Multiple replicas are safe. Claims are atomic inside the adapter — one
// statement takes the rows and their lease (postgres uses FOR UPDATE SKIP
// LOCKED) — so replicas poll the same table without stepping on each other.
// The lease bounds the damage when a replica dies mid-publish: the rows become
// claimable again once `outbox.lock_seconds` expires.
//
// A store that does not implement `outbox.PublisherSetter` (the in-memory
// adapter, whose Record publishes synchronously) is rejected with
// PublisherSetterError rather than started without a destination.
func (c *Container) OutboxRelay(ctx context.Context) (outbox.Store, error) {
	store, err := c.Outbox()
	if err != nil {
		return nil, fmt.Errorf("container: outbox relay: resolve outbox: %w", err)
	}

	setter, ok := store.(outbox.PublisherSetter)
	if !ok {
		return nil, PublisherSetterError{Actual: fmt.Sprintf("%T", store)}
	}

	publisher, err := c.outboxPublisher()
	if err != nil {
		return nil, err
	}

	setter.SetPublisher(publisher)

	if err := store.Start(ctx); err != nil {
		return nil, fmt.Errorf("container: outbox relay: start: %w", err)
	}

	return store, nil
}

// outboxPublisher builds the relay's destination from the transport selector.
// Resolving the selected transport is a side effect, the same way `Job()`
// resolves `Queue()`: the relay holds a live reference for as long as it runs.
func (c *Container) outboxPublisher() (outbox.Publisher, error) {
	if c.cfg.Outbox.Options.Publisher == outbox.PublisherEventBus {
		bus, err := c.EventBus()
		if err != nil {
			return nil, fmt.Errorf("container: outbox relay: resolve eventbus: %w", err)
		}

		publisher, err := outboxbridge.EventBusPublisher(bus)
		if err != nil {
			return nil, fmt.Errorf("container: outbox relay: %w", err)
		}

		return publisher, nil
	}

	q, err := c.Queue()
	if err != nil {
		return nil, fmt.Errorf("container: outbox relay: resolve queue: %w", err)
	}

	publisher, err := outboxbridge.QueuePublisher(q)
	if err != nil {
		return nil, fmt.Errorf("container: outbox relay: %w", err)
	}

	return publisher, nil
}
