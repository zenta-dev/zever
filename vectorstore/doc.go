// Package vectorstore stores embeddings and runs similarity queries with swappable adapters.
//
// It upserts vectors, deletes by ID, and queries top matches by cosine similarity only. It is not a full-text index and does not do hybrid ranking itself.
//
// Type safety: VectorStore plus typed Options plus Adapter enum plus Factory. Vector plus ScoreMatch are typed; defaults are DefaultDimension 1536 and DefaultTopK 10 and DefaultSQLiteDSN memory. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env VECTORSTORE_<FIELD> (no prefix, e.g. VECTORSTORE_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.VectorStore(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error with no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed vectorstore:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. APIKey is never logged and empty embeddings are rejected before IO.
//
// Performance: queries return up to topK ordered by descending score with topK <= 0 meaning 10. UpsertBatch writes many vectors in one call instead of one round trip per vector; adapters implement it as a single multi-row statement, a native batch API, or a transaction wrapping the single-item path. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package vectorstore
