// Package lrucache provides a shared generic LRU cache, plus a TTL-wrapping
// variant, so adapter batteries stop hand-rolling their own
// container/list-backed eviction.
//
// Beyond the basic Get/Put/Delete/Len surface, Cache.GetOrCompute offers an
// atomic check-and-insert for avoiding duplicate work on a concurrent cache
// miss, and Cache.Clear offers a bulk-teardown path with its own per-entry
// callback (distinct from WithOnEvict). TTLCache.PutTTL lets a single
// instance serve entries with different, per-call TTLs, matching the
// positive/negative-caching split already needed by i18n/remote/remote.go.
package lrucache
