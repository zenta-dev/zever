// Package document renders document sources with swappable adapters.
//
// It renders byte sources to PDF, PNG, or JPG through local, remote, or LaTeX backends. It is not a document editor or storage layer; inputs are bounded byte slices.
//
// Type safety: Document plus typed Options plus Adapter enum plus Factory. OutputFormat is a string type limited to pdf, png, and jpg; Quality is clamped to 0-100. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env DOCUMENT_<FIELD> (no prefix, e.g. DOCUMENT_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Document(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Document releases its backend with Close() which takes no ctx; Render returns nil output on error.
//
// Errors: sentinel errors, errors.Is compatible, prefixed document:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. APIKey is never logged, endpoint URLs are validated, and LaTeX commands come from fixed config only.
//
// Performance: default 30s timeout with 10MiB source and 64MiB output caps; intermediate artifacts spill to TmpDir. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package document
