// Package postgres provides a DB-backed workflow.Workflow implementation
// whose run state survives process restarts.
//
// Runs live in one table (default workflow_runs); every state transition
// goes through the orm typed builder, so in-flight runs are recoverable
// after a restart or deploy by any replica holding the same DSN. Crash
// recovery uses lease_owner/lease_expires_at columns: a second owner
// reclaims an expired lease and re-executes the step, mirroring the
// claim/reclaim shape of adapters/queue/redis (see Reclaimer).
//
// Durability boundary: the database persists run inputs, step results, and
// leases. Step functions themselves are process-local code registered via
// workflow.StepRegistrar; every replica that may reclaim work must register
// the same step names, and steps must be idempotent because a reclaim
// re-executes them.
//
// Core stays frozen: this package defines its own Options (DSN plus pool
// knobs) and the Reclaimer extension interface instead of changing
// core/workflow. Container wiring shares one pool per exact DSN across
// batteries (see container/pools.go); dedicated_pool: true restores a
// private pool.
package postgres
