// Package authz gates HTTP and gRPC handlers with composable adapters.
//
// It owns bearer token extraction, the Policy gate and the
// auth.Auth-to-permission.Checker Authorize flow. It issues no tokens and
// stores no permissions itself.
//
// Type safety: Policy plus Authorize plus Middleware and
// UnaryServerInterceptor helpers. UnauthenticatedError and
// PermissionDeniedError are typed. Unsupported features fail closed.
//
// DX: Open with Authorize for direct checks, Middleware for HTTP and
// UnaryServerInterceptor for gRPC; there is no Open or Register surface.
// Options are typed with zero-infra defaults for tests. No service env
// applies; see config/README.md.
//
// Container: no container accessor exists; compose auth.Auth and
// permission.Checker directly. Lazy per-service singleton, retry on error. See
// container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. There is no Close or Stop shape in this
// package; close the underlying auth and permission backends.
//
// Errors: sentinel errors, errors.Is compatible, prefixed authz:. Name service
// and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Token bytes never appear in errors or logs; failure reasons stay generic so
// callers cannot probe credential validity.
//
// Performance: fast path returns before any backend call when no auth or check
// is required; metadata parsing avoids allocations on empty input. Bounded
// pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package authz
