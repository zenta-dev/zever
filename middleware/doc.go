// Package middleware provides HTTP handling with swappable resolved backends.
//
// It ships recovery, structured access logging, rate limiting, timeouts, and
// tracing as HTTP plus unary-gRPC pairs with matching semantics. It does not
// own an adapter registry or config section, it wraps already-resolved
// instances.
//
// Type safety: plain constructor functions over resolved log, ratelimit, and
// observability instances plus FailMode enum plus Option. There is no Adapter
// enum or Factory and no Open for this package. Unsupported features fail
// closed with fixed error envelopes.
//
// DX: plain constructors over resolved instances instead of a registry:
// RequestLogger, Recover, RateLimit, Timeout, and Tracing plus their
// UnaryServerInterceptor counterparts, with RemoteAddrKey and PeerAddrKey for
// keying. No adapter name to parse and no middleware section in config files.
// See config/README.md.
//
// Chain order: Recover outermost, so it catches panics from every other
// middleware and from a Timeout-abandoned handler still running in the
// background; Timeout next, so it bounds everything inside it (including
// RateLimit's own limiter call); RateLimit inside that, so a denied request
// never starts the timeout clock's downstream work.
//
// Container: no dedicated accessor, construct over container-resolved
// instances after container.New(cfg) such as c.Log() and c.Ratelimit(). Lazy
// per-service singleton, retry on error applies to those services. See
// container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Constructors return
// http.Handler wrappers and grpc.UnaryServerInterceptor values with no Close
// or Stop. Handlers use r.Context and the gRPC ctx directly. Only resolved
// services close via the container.
//
// Errors: sentinel errors are not used here, errors surface as fixed shapes.
// HTTP denials use the {"error": "..."} envelope with 500 or 429, gRPC uses
// codes.Internal or codes.ResourceExhausted. Name service and field only in
// errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Only the request path is logged, never the query string. Panic values and
// stacks go to logs only while clients get a fixed message.
//
// Performance: one shared statusRecorder per request with no double wrap.
// RemoteAddrKey splits host-port once per request. Metrics counters are
// best-effort and never fail a request. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see ExampleRequestLogger in example_test.go.
package middleware
