// Package eventbus publishes topic messages with swappable adapters.
//
// It delivers each message to push handlers or buffered pull channels with drop-newest backpressure. It is not a durable log or task queue; use queue or job for durable work.
//
// Type safety: Eventbus extends Pusher plus typed Options plus Adapter enum plus Factory. Message carries a UUIDv7 ID, topic, typed Payload and Headers with Clone, and a ReceivedAt publish stamp; Wrap upgrades any Pusher with pull support. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env EVENTBUS_<FIELD> (no prefix, e.g. EVENTBUS_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Eventbus(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Push Subscribe returns an unsubscribe func; pull SubscribeChan returns a channel closed by Unsubscribe or Close, and Close is idempotent.
//
// Errors: sentinel errors, errors.Is compatible, prefixed eventbus:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Redis addr and prefix are validated and payload content never appears in errors; topics are capped at 256 bytes.
//
// Performance: per-topic buffers default to 1024 with 128 max handlers, 30s handler timeout, 5s close timeout, and 4MiB payload cap. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package eventbus
