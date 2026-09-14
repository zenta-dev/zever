// Package memory provides an in-process idempotency store.
//
// Entries live in a mutex-guarded map with an absolute expiry stamp.
// Expiry is enforced lazily on access plus an opportunistic full-map
// sweep on every Begin, so there is no background goroutine to manage.
// The O(n) sweep is acceptable for process-local use; beyond ~10k keys
// prefer the redis adapter.
package memory
