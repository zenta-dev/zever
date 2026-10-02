// Package memory provides an in-process lock adapter, the zero-infra
// default for distributed locking.
//
// Leases live in a shared/lrucache TTLCache keyed by lock key, each value
// carrying the holder's id with the lease TTL as the entry TTL. Expiry is
// enforced on every access rather than by a background goroutine: an entry
// whose deadline has passed counts as absent, so a holder that never unlocks
// (a crashed worker, a panicking handler) can never wedge the key permanently.
// Extend and Unlock use holder-equality CAS ops, so a stale handle never
// steals or releases a successor's lease.
package memory
