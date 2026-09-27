// Package router HTTP router facade with swappable adapters.
//
// It registers Handle routes, Group scopes, and Use middleware with uniform
// Param access and NormalizePattern conversion across backends. It ships no
// middleware implementations and it runs no listen loop; adapters own that.
//
// Type safety: Router plus Group plus typed Options plus Adapter enum plus Factory.
// ValidMethod gates 9 standard methods, and NormalizePattern plus ChiPattern
// convert brace and colon params with MalformedPatternError detail. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env ROUTER_<FIELD> (no prefix, e.g. ROUTER_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Router(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Router exposes no Close; registration takes no ctx and request params travel on the request context via WithParams.
//
// Errors: sentinel errors, errors.Is compatible, prefixed router:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. AppName caps at 64 characters with no control bytes, stored params are copied to block aliasing, and untrusted patterns are never compiled as regexps.
//
// Performance: braceParamRe compiles once, builders pre-grow, normalization runs at registration not per request. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package router
