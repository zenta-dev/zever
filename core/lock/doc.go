// Package lock provides distributed locking with swappable adapters.
//
// It covers single-shot TryAcquire, blocking Acquire, lease Extend, and Unlock
// with ownership checks so release never steals another holder. It does not
// manage job queues or general caching, only TTL-bounded leases.
//
// Type safety: Locker plus Lock interfaces with typed Options
// plus Factory. TTL and RetryInterval are typed durations with memory and redis
// backends. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it. Options are typed with
// zero-infra defaults for tests: empty Options falls back to DefaultTTL and
// DefaultRetryInterval, and the memory adapter needs no server. Configuration
// comes from the environment only: .env, .env.local, and the process
// environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Lock(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. TryAcquire and Acquire take
// ctx plus key plus ttl, Extend and Unlock take ctx, and Close takes ctx.
// Close releases adapter resources; it does not release caller-held leases,
// those expire on their own TTL. Only resolved services close.
//
// Errors: sentinel errors, errors.Is compatible, prefixed lock:. Name service
// and field only in errors, with InvalidAdapterError, DuplicateError, and
// UnknownAdapterError carrying the adapter.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Redis passwords stay in Options and support TLS plus RequireTLS against
// plaintext connections. Validate keys and TTLs.
//
// Performance: blocking Acquire polls every RetryInterval defaulting to 50ms.
// Leases are bounded by TTL defaulting to 30s. TryAcquire is a single attempt
// with no retry loop. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see Example in example_test.go.
package lock
