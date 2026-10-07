// Package outboxbridge adapts a resolved messaging transport into the
// core/outbox.Publisher the outbox relay publishes through.
//
// core/outbox is transport-agnostic on purpose: Options carries only the
// informational selector string ("queue" or "eventbus"), never a live
// transport. This package supplies the missing edge, turning a resolved
// core/queue.Queue or core/eventbus.EventBus into a Publisher. It exists as its
// own module so core/outbox never depends on the queue or eventbus batteries
// and those batteries never depend on outbox.
//
// DX: QueuePublisher and EventBusPublisher are the two entry points;
// container.OutboxRelay already calls them, so applications wiring the relay
// by hand use the same pair. A nil transport is an error, not a publisher that
// fails every publish.
//
// Mapping: outbox.Message.Topic becomes the transport topic, Payload the
// payload bytes, and Headers the transport headers. Neither transport carries a
// routing/partition key field, so Message.Key travels under the KeyHeader
// header (see that constant for the exact contract).
//
// Container: no container accessor of its own; container.OutboxRelay is the
// caller. See container/README.md.
//
// Lifecycle: no IO, no background work, no state; the returned Publisher holds
// only the transport reference it was given.
//
// Errors: a nil queue or eventbus fails with ErrNilQueue / ErrNilEventBus, and
// a transport error is wrapped with the message ID and topic. Errors are never
// swallowed: the relay turns a non-nil error into a retry.
//
// Security: headers are copied, never mutated in place, and nothing is logged.
//
// Concurrency: safe for concurrent use; the publishers hold no mutable state.
// No globals, no init wiring.
package outboxbridge

import (
	"context"
	"fmt"
	"maps"

	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/queue"
)

// KeyHeader is the header name carrying outbox.Message.Key. Neither
// queue.Push nor eventbus.Publish takes a routing or partition key, so the key
// rides along as a header instead of being dropped. It wins over a recorded
// header of the same name, because Message.Key is the authoritative field.
// Consumers read it back with the Key accessor.
const KeyHeader = "outbox-key"

// Key returns the routing key carried by headers, or "" when the transport
// carried none. It is the inverse of the KeyHeader mapping for consumers that
// need the key back out of the headers.
func Key(headers map[string]string) string {
	if headers == nil {
		return ""
	}

	return headers[KeyHeader]
}

// queuePublisher publishes through a core/queue.Queue.
type queuePublisher struct {
	q queue.Queue
}

// eventBusPublisher publishes through a core/eventbus.EventBus.
type eventBusPublisher struct {
	bus eventbus.EventBus
}

var (
	_ outbox.Publisher = queuePublisher{}
	_ outbox.Publisher = eventBusPublisher{}
)

// QueuePublisher adapts a resolved queue.Queue into an outbox.Publisher. Each
// published message is pushed onto msg.Topic. A nil queue fails with
// ErrNilQueue.
func QueuePublisher(q queue.Queue) (outbox.Publisher, error) {
	if q == nil {
		return nil, ErrNilQueue
	}

	return queuePublisher{q: q}, nil
}

// EventBusPublisher adapts a resolved eventbus.EventBus into an
// outbox.Publisher. Each published message is delivered to msg.Topic. A nil
// bus fails with ErrNilEventBus.
func EventBusPublisher(bus eventbus.EventBus) (outbox.Publisher, error) {
	if bus == nil {
		return nil, ErrNilEventBus
	}

	return eventBusPublisher{bus: bus}, nil
}

// Publish pushes msg onto its topic as a queue message. A transport failure is
// wrapped with the message ID and topic, so the relay records an actionable
// last_error instead of a bare transport string.
func (p queuePublisher) Publish(ctx context.Context, msg outbox.Message) error {
	if err := p.q.Push(ctx, msg.Topic, queue.Payload(msg.Payload), headers(msg)); err != nil {
		return fmt.Errorf("outboxbridge: push %q to topic %q: %w", msg.ID, msg.Topic, err)
	}

	return nil
}

// Publish delivers msg to its topic subscribers. A transport failure is
// wrapped with the message ID and topic, so the relay records an actionable
// last_error instead of a bare transport string.
func (p eventBusPublisher) Publish(ctx context.Context, msg outbox.Message) error {
	if err := p.bus.Publish(ctx, msg.Topic, eventbus.Payload(msg.Payload), headers(msg)); err != nil {
		return fmt.Errorf("outboxbridge: publish %q to topic %q: %w", msg.ID, msg.Topic, err)
	}

	return nil
}

// headers builds the transport headers for msg: the recorded headers copied
// into a fresh map, plus the routing key under KeyHeader when set. The copy
// keeps the message's own map unaliased, so a transport that retains or mutates
// the headers cannot corrupt a message the relay still holds. A message with
// neither headers nor a key carries nil, so transports that special-case an
// empty map see no map at all.
func headers(msg outbox.Message) map[string]string {
	if msg.Key == "" && len(msg.Headers) == 0 {
		return nil
	}

	out := make(map[string]string, len(msg.Headers)+1)
	maps.Copy(out, msg.Headers)

	if msg.Key != "" {
		out[KeyHeader] = msg.Key
	}

	return out
}
