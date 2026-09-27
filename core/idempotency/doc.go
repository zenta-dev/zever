// Package idempotency guards retried executions with swappable adapters.
//
// It reserves a caller key with Begin and records the result with Complete so retries replay stored output. It is not a lock manager or transaction coordinator; all errors are fail-closed.
//
// Type safety: Store plus BeginOptions plus Outcome plus typed Options plus Adapter enum plus Factory. Keys are capped at 255 bytes and fingerprints match under FingerprintMatches semantics. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env IDEMPOTENCY_<FIELD> (no prefix, e.g. IDEMPOTENCY_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Idempotency(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Store releases its backend with Close() which takes no ctx; Forget is idempotent and returns nil on missing keys.
//
// Errors: sentinel errors, errors.Is compatible, prefixed idempotency:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Key bytes are never echoed in errors, length only, and per-call fingerprints are size-capped.
//
// Performance: memory store is a mutex-guarded map with 24h default TTL; keys cap at 255 bytes and fingerprints at 4096. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package idempotency
