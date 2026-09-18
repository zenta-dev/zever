// Package observability telemetry facade with swappable adapters.
//
// It carries traces and metrics through Provider, Tracer, Metrics, and Span
// with typed Attr values. It does not run a collector and it does not wire
// a global SDK behind the caller's back.
//
// Type safety: Provider plus typed Options plus Adapter enum plus Factory. Attr
// values use the sealed AttributeValue set with String, Int64, Float64, and
// Bool constructors, bounded by MaxKeyLen 256, MaxValueLen 4096, and MaxAttrs
// 32. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env OBSERVABILITY_<FIELD> (no prefix, e.g. OBSERVABILITY_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Observability(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. There is no Close here; Shutdown on Provider, Tracer, and Metrics flushes buffered telemetry.
//
// Errors: sentinel errors, errors.Is compatible, prefixed observability:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Insecure mode is loopback-only, CA and cert plus key must pair, authorization headers need secure mode, and sensitive attribute keys are redacted.
//
// Performance: SampleRatio in [0,1] skips work early, AttrValueLimit 0 or in [256,16384] caps strings, headers capped at 32. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package observability
