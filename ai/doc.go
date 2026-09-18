// Package ai provides LLM generation, streaming and embeddings with swappable adapters.
//
// It owns the Generate, Stream and Embed facade plus typed message and tool
// payloads. It does not run servers or manage credentials beyond passing
// Options to adapters.
//
// Type safety: AI plus typed Options plus Adapter enum plus Factory. Role,
// ToolChoice, GenerateOptions, Generation, Usage, EmbedOptions and
// StreamChunk are typed. Unsupported features fail closed with ErrNotSupported.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests. Config file plus env AI_<FIELD> (no prefix,
// e.g. AI_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.AI(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. AI closes with Close() error and no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed ai:. Name service
// and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// APIKey values never appear in errors or logs; BaseURL must use https scheme.
//
// Performance: Timeout bounds provider calls with adapter defaults on zero.
// Streams use bounded channels closed by the producer. Bounded pools and
// timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package ai
