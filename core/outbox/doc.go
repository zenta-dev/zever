// Package outbox defines the transactional outbox/inbox battery.
//
// A transactional outbox fixes the dual-write hazard: the business row and
// the event it should emit are written in one database transaction through
// Store.Record, so either both commit or neither does. A relay then
// publishes the stored event with at-least-once delivery. An inbox makes
// consumers idempotent by recording each event ID in the same transaction
// as the side effect.
//
// Core stays transport-agnostic: it depends only on core/db for the
// transaction type, core/observability for relay telemetry, and
// shared/registry, shared/retry, and shared/traceprop for wiring. It never
// imports core/eventbus or core/queue; application wiring bridges a concrete
// transport to the Publisher interface.
//
// Adapters live one module each under adapters/outbox: db (default, durable
// sqlite/postgres polling) and memory (dev/test). The outboxtest package
// provides the conformance kit every adapter runs.
package outbox
