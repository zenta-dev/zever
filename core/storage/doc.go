// Package storage mints object URLs and manages objects with swappable adapters.
//
// It presigns uploads and downloads and supports exists, delete, and move with bucket policy helpers. It is not a CDN and does not serve bytes itself.
//
// Type safety: Storage plus typed Options plus Factory. Options embed LocalOptions plus S3Options plus R2Options with URLBase and Policy; bucket and key are validated. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Storage(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close(ctx context.Context) error.
//
// Errors: sentinel errors, errors.Is compatible, prefixed storage:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. HMAC secrets are never logged and presigned TTL is bounded by MaxPresignTTL.
//
// Performance: presign TTL must be positive and at most 7 days. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package storage
