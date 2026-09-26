// Package apperror bridges business logic to transports with swappable adapters.
//
// It owns the closed ErrorCode vocabulary plus the sealed Error type and its
// HTTP status mapping. It does not touch net/http or gRPC itself; transport
// wiring maps codes via errors.As.
//
// Type safety: Error plus typed ErrorCode plus New and Wrap constructors. The
// 16 ordinals mirror gRPC codes 1 through 16; out-of-range codes fail closed
// to UNKNOWN. Unsupported features fail closed.
//
// DX: Open with New for plain errors and Wrap for chained causes; there is no
// Open or Register surface. Options are typed with zero-infra defaults for
// tests. No service env applies; see config/README.md.
//
// Container: no container accessor exists; import apperror directly. Lazy
// per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: no IO and no ctx; values are immutable after construction. Close
// releases resources; only resolved services close. There is no Close or Stop
// shape in this package.
//
// Errors: sentinel errors, errors.Is compatible, prefixed apperror:.
// Discrimination is code-based via Code, so errors.Is matches only
// caller-supplied causes. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Messages carry only caller-supplied text; causes stay behind Unwrap.
//
// Performance: allocation is one struct plus optional cause with no pooling
// needs. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleNew in example_test.go.
package apperror
