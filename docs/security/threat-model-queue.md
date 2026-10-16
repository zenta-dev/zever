# Threat model: queue

Audited against `core/queue/` (`queue.go`, `message.go`, `options.go`) and
`adapters/queue/memory`, `adapters/queue/redis` at WS6 time.

## Assets

- Message payloads (often PII, credentials for downstream calls, or
  signed webhooks) and delivery state (visibility/inflight tracking).
- Worker capacity: a poison message that crashes every consumer is a
  queue-wide DoS.

## Trust boundaries

- Producers and consumers may live in different processes/trust domains but
  share topics. Payloads are opaque bytes: the queue never inspects,
  validates, or sanitizes them.
- The Redis URL crosses config → adapter with credentials.

## What zever guarantees

- Explicit lifecycle contract: `Push`/`PushDelayed`/`Pop`/`Ack`/`Nack(requeue)`/
  `Length`/`IsEmpty`/`Close`/`Name`. Redelivery on `Nack(requeue=true)` or
  visibility timeout is part of the contract, not an accident.
- `Pop` clones isolate consumer mutations from inflight state (pinned by
  `TestMemory_CloneIsolation`); the memory adapter's delay/visibility
  structures are timing machinery using non-security randomness where noted.
- Redis DSN redaction is covered by fixture tests
  (`adapters/queue/redis/redis_test.go`).

## What the caller must do

- Make every handler **idempotent**: redelivery is guaranteed possible, so
  at-least-once processing must be safe. Dedupe by message/event id where
  the domain needs exactly-once effects (see `payment.WebhookHandling` for
  the established pattern).
- `Ack` only after durable effect; `Nack(requeue=true)` only for retryable
  failures. Dead-letter or drop poison messages after a bounded retry count
  — infinite requeue is a self-inflicted DoS.
- Validate and size-limit payloads in the handler; the queue will happily
  deliver a 100 MB payload or a malicious shape to a naive consumer.
- Keep the Redis URL secret; scope topics so one tenant cannot read or
  poison another's.

## Explicit non-guarantees

- No payload encryption, signing, or schema validation in transit. End-to-end
  authenticity of cross-domain messages is caller-owned (sign at produce,
  verify at consume).
- No ordering guarantees beyond what each adapter documents; do not build
  security sequencing (e.g. "revoke always processed before issue") on
  assumed FIFO.
