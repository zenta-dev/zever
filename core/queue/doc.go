// Package queue topic-based message queue facade with swappable adapters.
//
// It pushes, delays, pops, acks, nacks, and measures topics through Queue with
// typed Message, Payload, Headers, and MessageID values. It promises no
// ordering or exactly-once delivery beyond what the backend provides.
//
// Type safety: Queue plus typed Options plus Adapter enum plus Factory. MessageID
// is a UUIDv7 value with ParseMessageID validation, and Payload and Headers
// offer Clone helpers. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env QUEUE_<FIELD> (no prefix, e.g. QUEUE_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Queue(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Queue exposes Close() error, and Pop reports ErrEmpty or EmptyError when no message is ready.
//
// Errors: sentinel errors, errors.Is compatible, prefixed queue:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Message IDs validate as UUIDs, empty-queue errors name only the topic, and Redis keys stay under a configured prefix.
//
// Performance: Buffer caps ready messages per topic, VisibilityTimeout and PollTimeout bound delivery waits with memory defaults of 30 and 5 seconds. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package queue
