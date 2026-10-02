// Package lrucache provides a shared generic LRU cache, plus a TTL-wrapping
// variant, so adapter batteries stop hand-rolling their own
// container/list-backed eviction.
//
// Beyond the basic Get/Put/Delete/Len surface, Cache.GetOrCompute offers an
// atomic check-and-insert for avoiding duplicate work on a concurrent cache
// miss, and Cache.Clear offers a bulk-teardown path with its own per-entry
// callback (distinct from WithOnEvict). TTLCache.PutTTL lets a single
// instance serve entries with different, per-call TTLs, matching the
// positive/negative-caching split already needed by i18n/remote/remote.go;
// a non-positive TTL stores a persist entry with no expiry. TTLCache
// additionally offers the atomic primitives a core/cache adapter needs
// without a side structure: GetWithExpiry, SetIfAbsent, CompareAndDelete,
// CompareAndSwap (expiry-preserving, for counters), CompareAndExtend, plus
// PurgeExpired for janitors and Range for snapshots. Close/ErrClosed stay in
// the adapter: this package owns no goroutine and no closed state.
package lrucache
