// Package tenant resolves tenant identity with swappable adapters.
//
// It resolves tenant IDs from metadata and scopes contexts to a tenant. It is not an auth gate and does not enforce permissions itself.
//
// Type safety: Tenant plus typed Options plus Adapter enum plus Factory. Options carry Header plus SubdomainRegex plus ID with MaxRegexLength 500; defaults are DefaultHeader and DefaultSingleID. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env TENANT_<FIELD> (no prefix, e.g. TENANT_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Tenant(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error with no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed tenant:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Header keys are validated as printable ASCII and regex length is bounded.
//
// Performance: single backend is a constant lookup and header backend is a map read. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package tenant
