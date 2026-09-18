// Package secrets manages secret values with swappable adapters.
//
// It gets, sets, deletes, and lists named secrets with strict name validation. It is not a config loader and the env backend is read-only.
//
// Type safety: Secrets plus typed Options plus Adapter enum plus Factory. Options carry Addr plus Token plus Mount plus ProjectID plus Prefix plus Region; ValidateName enforces safe names. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Env backend reads prefixed variables directly (no prefix, e.g. PREFIX_NAME). See config/README.md.
//
// Container: not managed by container; there is no c.Secrets accessor. Call Open directly with typed Options. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close(ctx context.Context) error.
//
// Errors: sentinel errors, errors.Is compatible, prefixed secrets:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Values are never included in errors and names reject slash and dot-dot traversal.
//
// Performance: env reads are process-local lookups with no network. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package secrets
