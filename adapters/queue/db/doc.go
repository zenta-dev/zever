// Package db provides a DB-backed queue.Queue over postgres or sqlite.
//
// Messages live in a single table (one row per message) with a topic
// column, so every topic shares the claim machinery and per-topic
// isolation falls out of the WHERE clause. Delayed delivery is an
// available_at timestamp: rows become claimable when it passes, the same
// shape as shared/kvstore's SweepExpired promotion. Crash recovery is a
// lease on each row (claimed_by/claimed_until): Pop claims the oldest
// ready row with a compare-and-set UPDATE guarded on the previously read
// lease, bumps attempt on reclaim, and Ack/Nack settle the row under an
// (id, attempt) guard, mirroring adapters/queue/redis's claim/reclaim/
// ack/nack Lua scripts and adapters/workflow/postgres's lease CAS.
//
// Honest trade-off: this is a polling transport with moderate throughput,
// not a Kafka/SQS replacement. Every Pop polls on PollInterval, claims
// serialize on one row per topic head, and reclaim sweeps at most every
// sweepInterval. It fits the same niche as Rails' Solid Queue: durable
// background jobs on a database you already run (a separate queue database
// with 3-5 connections per worker is recommended there, and applies here
// too). Throughput tops out in the low thousands of jobs per second per
// topic; beyond that use redis or a dedicated broker.
//
// Pool sharing: the container resolves one pool per DSN and shares it
// across every battery pointing at the same DSN, so a queue on the app
// database borrows that pool instead of opening its own. Set
// dedicated_pool: true (ideally with a separate queue database) to restore
// a private pool under sustained load.
package db
