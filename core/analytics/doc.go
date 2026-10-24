// Package analytics provides event tracking with swappable adapters.
//
// It owns the Track, Identify and Group facade plus property bound checks and
// context identity helpers. It does not store user profiles or run pipelines.
//
// Type safety: Analytics plus typed Options plus Factory.
// CountLimitError and SizeLimitError carry observed counts and limits.
// Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with
// zero-infra defaults for tests. Configuration comes from the environment
// only: .env, .env.local, and the process environment, with keys named
// <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Analytics(). Lazy per-service
// singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. Analytics closes with Close() error and no
// ctx; call it before process exit to flush buffered events.
//
// Errors: sentinel errors, errors.Is compatible, prefixed analytics:. Name
// service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// APIKey values never appear in errors or logs; log adapter output carries no
// PII redaction so use it for debug output only.
//
// Performance: MaxProperties and MaxPropertiesBytes bound payload size before
// send; zero means package defaults. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package analytics
