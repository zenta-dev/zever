// Package resilience composes process-local reliability policies with swappable adapters.
//
// A Manager hands out named per-dependency Guard values. Each Guard.Execute
// wraps a function with an outermost-to-innermost policy chain:
// Timeout, then Retry, then circuit Breaker, then Bulkhead, then the function.
// The Timeout therefore governs the whole operation including retries.
//
// Adapters: the memory adapter keeps every policy process-local. The redis
// adapter (adapters/resilience/redis) shares circuit-breaker state across
// replicas via Redis while timeout, retry, and bulkhead stay process-local,
// because only breaker state is meaningful to share.
//
// Type safety: Guard, Manager, typed Options, and Factory.
// Breaker cancellation from the caller is never counted against the circuit,
// and callers classify business errors as success through BreakerOptions.IsSuccessful.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it.
// zero-infra defaults for tests. Configuration comes from the environment
// only: .env, .env.local, and the process environment, with keys named
// <SERVICE>_<FIELD> (no prefix). See
// config/README.md.
//
// Container: container.New(cfg) then c.Resilience(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. Guard.Close and Manager.Close are idempotent.
//
// Errors: sentinel errors, errors.Is compatible, prefixed resilience:.
// ErrOpenState, ErrTooManyRequests, ErrBulkheadFull, and ErrTimeout are the
// runtime policy outcomes; the remaining sentinels cover registration and options.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Options carry no credentials.
//
// Performance: DefaultTimeout is 5 seconds, DefaultMaxConcurrent is 100, and
// DefaultMaxWait is 1 second. No globals, no init wiring.
//
// Concurrency: safe for concurrent use. A Manager and its Guards are goroutine-safe.
//
// Example: see ExampleDo in example_test.go.
package resilience
