# Threat model: cache

Audited against `core/cache/` (`cache.go`, `options.go`, `typed.go`) and
`adapters/cache/memory`, `adapters/cache/redis` at WS6 time.

## Assets

- Cached values, which routinely include session-adjacent data, rendered
  fragments with PII, and rate-limit/counter state. The cache is also a
  poisoning target: a forged entry is served as truth until TTL.

## Trust boundaries

- Cache keys and values cross caller → backend (memory or Redis over the
  network). The Redis URL (with credentials) crosses config → adapter.
- TTL/expiry is advisory freshness, not a security boundary.

## What zever guarantees

- Small explicit contract (`Get`/`Set`/`SetIfAbsent`/`Delete`/`Increment`/
  `Decrement`/`Exists`/`Close`): no hidden reads, no secondary indexes, no
  cross-key transactions for callers to misunderstand.
- Redis addresses and key prefixes are validated at `Options.Validate`
  (`shared/redisopt`); DSN-shaped URLs are scrubbed in redacted display, and
  redaction is covered by fixture tests (`adapters/cache/redis/redis_test.go`).
- The memory adapter's expiry jitter uses non-cryptographic randomness
  explicitly marked `//nolint:gosec // weak rand is fine for jitter`
  (`adapters/cache/memory/memory.go`): jitter is timing noise, never a secret.

## What the caller must do

- Namespace keys per tenant/purpose; never let one tenant write another
  tenant's key. Treat cache as untrusted readable storage: do not place
  secrets in it unencrypted, and re-validate authorization on cache hits
  (a hit must not bypass access checks).
- Set TTLs deliberately; do not use the cache as durable storage — entries
  vanish on restart (memory) or eviction, and stale reads are normal.
- Keep the Redis URL in secrets management; require TLS/auth to Redis in
  production. Use `SetIfAbsent` for claim-once semantics, not check-then-set.

## Explicit non-guarantees

- No encryption at rest, no per-tenant isolation, no cache-poisoning defense
  beyond TTL: anyone who can write a key controls what readers see under it.
- Atomicity covers single counters only; multi-key consistency is not offered.
