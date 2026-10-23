// Package eventbus publishes topic messages with swappable adapters.
//
// It delivers each message to push handlers or buffered pull channels with drop-newest backpressure. It is not a durable log or task queue; use queue or job for durable work.
//
// Type safety: EventBus extends Pusher plus typed Options plus Factory. Message carries a UUIDv7 ID, topic, typed Payload and Headers with Clone, and a ReceivedAt publish stamp; Wrap upgrades any Pusher with pull support. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.EventBus(). Lazy per-service singleton, retry on error. See container/README.md.
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
// Example: see Example in example_test.go.
package eventbus
