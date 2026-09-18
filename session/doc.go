// Package session manages server-side sessions with swappable adapters.
//
// It creates, reads, saves, and deletes sessions with TTL expiry; expired sessions read as missing. It is not a cookie jar and does not mint client tokens itself.
//
// Type safety: Store plus typed Options plus Adapter enum plus Factory. Session plus NewSession plus Clone plus Expired are typed; zero TTL means DefaultTTL of 15 minutes. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env SESSION_<FIELD> (no prefix, e.g. SESSION_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Session(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error and it stops the memory sweep goroutine.
//
// Errors: sentinel errors, errors.Is compatible, prefixed session:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. IDs are secrets and are never echoed; InvalidIDError reports length only.
//
// Performance: memory sweep runs at ttl divided by 2 clamped to 1s to 5m. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package session
