// Package cdc provides a transactional outbox.Store backed by PostgreSQL
// logical replication (change data capture).
//
// Record emits the message with pg_logical_emit_message inside the caller's
// transaction, so the event and the business row commit together; a rollback
// drops the event. The consumer tails the write-ahead log through a logical
// replication slot (pgoutput plugin), filters messages by prefix, publishes
// each matching message through the configured outbox.Publisher, and
// acknowledges the LSN only after a successful publish. Delivery is
// at-least-once: a message whose publish exhausts the attempt budget is left
// unacknowledged so a later run redelivers it.
//
// The adapter requires wal_level=logical on the server. Live behavior is
// covered by tests gated behind POSTGRES_DSN.
package cdc
