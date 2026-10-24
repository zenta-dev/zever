// Package search provides full-text search with swappable adapters.
//
// It indexes documents and runs ranked queries with explicit index filters and paged results. It is not a vector store and does not rank by embedding similarity.
//
// Type safety: Search plus typed Options plus Factory. Document plus QueryOptions plus Result plus Hit are typed; Limit <= 0 selects DefaultLimit 10. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Search(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error with no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed search:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. APIKey is never logged and Host must be a valid URL with scheme and host.
//
// Performance: paged queries with Total ignoring Limit and Offset and backend-defined scores. IndexBatch indexes many documents in one call instead of one round trip per document; adapters implement it as a single multi-row statement, a native bulk API, or a transaction wrapping the single-item path. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package search
