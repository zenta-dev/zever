// Package memory provides an in-process lock adapter, the zero-infra
// default for distributed locking.
//
// Leases live in a mutex-guarded map keyed by lock key, each carrying the
// holder's id and an absolute deadline. Expiry is enforced on every access
// rather than by a background goroutine: an entry whose deadline has passed
// is treated as free, so a holder that never unlocks (a crashed worker, a
// panicking handler) can never wedge the key permanently.
package memory
