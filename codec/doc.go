// Package codec encodes and decodes values with composable adapters.
//
// It owns the generics Encoder, Decoder and Codec interfaces plus the JSON
// implementation. It performs no IO and manages no connections.
//
// Type safety: Codec[V] plus Encoder[V] plus Decoder[V] plus JSONCodec[V].
// JSON output is deterministic for stable snapshots. Unsupported features fail
// closed.
//
// DX: Open with JSONCodec[V] directly; there is no Open or Register surface.
// Options are typed with zero-infra defaults for tests. No service env
// applies; see config/README.md.
//
// Container: no container accessor exists; use JSONCodec[V] directly. Lazy
// per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: no IO and no ctx; values are pure functions of their inputs.
// Close releases resources; only resolved services close. There is no Close or
// Stop shape in this package.
//
// Errors: sentinel errors, errors.Is compatible, prefixed codec:. Name service
// and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Encoded payloads may hold secrets, so never log encoded bytes; decode
// failures wrap the cause without echoing input.
//
// Performance: JSON uses deterministic marshaling with a single pass and no
// pooling needs. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleJSONCodec in example_test.go.
package codec
