// Package cache provides byte-slice entries, counters and leases with swappable adapters.
//
// It owns the Cache facade plus generics Typed helpers and the optional
// CompareAndSwapCache lease capability. It runs no servers and owns no
// serialization beyond Typed codecs.
//
// Type safety: Cache plus typed Options plus Adapter enum plus Factory.
// Typed[K, V] pairs Key constraints with codec.Codec values; CompareAndSwap
// stays a separate interface so third-party adapters keep working.
// Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests. Config file plus env CACHE_<FIELD> (no
// prefix, e.g. CACHE_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Cache(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. Cache closes with Close(ctx) error, taking
// the context unlike most backends.
//
// Errors: sentinel errors, errors.Is compatible, prefixed cache:. Name service
// and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Cached values may hold secrets, so never log keys or payloads; Redis URLs
// stay out of errors.
//
// Performance: memory backend bounds entries with MaxEntries and sweeps on
// SweepInterval; Redis options bound pool size, waits and connection lifetimes.
// Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package cache
