// Package resilience composes process-local reliability policies with swappable adapters.
//
// A Manager hands out named per-dependency Guard values. Each Guard.Execute
// wraps a function with an outermost-to-innermost policy chain:
// Timeout, then Retry, then circuit Breaker, then Bulkhead, then the function.
// The Timeout therefore governs the whole operation including retries.
//
// Type safety: Guard, Manager, typed Options, Adapter enum, and Factory.
// Breaker cancellation from the caller is never counted against the circuit,
// and callers classify business errors as success through BreakerOptions.IsSuccessful.
//
// DX: open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests. Config file plus env RESILIENCE_<FIELD>
// (no prefix, e.g. RESILIENCE_ADAPTER). See config/README.md.
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
