// Package storage mints object URLs and manages objects with swappable adapters.
//
// It presigns uploads and downloads and supports exists, delete, and move with bucket policy helpers. It is not a CDN and does not serve bytes itself.
//
// Type safety: Storage plus typed Options plus Adapter enum plus Factory. Options embed LocalOptions plus S3Options plus R2Options with URLBase and Policy; bucket and key are validated. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env STORAGE_<FIELD> (no prefix, e.g. STORAGE_ADAPTER). See config/README.md.
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
// Example: see ExampleOpen in example_test.go.
package storage
