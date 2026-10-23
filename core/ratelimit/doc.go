// Package ratelimit token-bucket rate limiter facade with swappable adapters.
//
// It allows or denies keyed requests through Limiter with Decision values
// carrying Allowed, RetryAfter, and Remaining. It is not HTTP middleware;
// callers map denials to 429 and choose fail-open behavior on error.
//
// Type safety: Limiter plus typed Options plus Factory. Options
// carry Rate per second and Burst bucket size, and ValidateCost plus ValidateKey
// share validation across adapters. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.RateLimit(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Limiter exposes Close() error which also stops the idle-entry sweeper.
//
// Errors: sentinel errors, errors.Is compatible, prefixed ratelimit:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. InvalidKeyError carries only the key length because keys may be PII, keys cap at MaxKeyLen 256 bytes with no control bytes, and token costs must be finite and positive.
//
// Performance: DefaultIdleTTL is 10 minutes, DefaultSweepInterval is 1 minute, charges cap at burst. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package ratelimit
