// Package grpcclient provides a gRPC client-connection helper with
// client-side load balancing, retry, tracing, and resilience.
//
// New constructs a *grpc.ClientConn from a target URL and functional
// options. The default service config enables round_robin load balancing;
// WithRetryPolicy merges a per-method retry policy (capped at
// DefaultMaxAttempts, gRPC's client max) into the same config. Targets
// under the dns:/// scheme use gRPC's built-in DNS resolver; targets under
// static:/// use a per-connection static resolver fed by
// WithStaticResolver, so no global resolver registration is involved.
//
// Every connection chains unary and stream client interceptors that, in
// order: start an observability span (when WithObservability is set),
// inject the W3C trace context from shared/traceprop into outgoing
// metadata, attach WithMetadata keys without overwriting existing ones,
// apply the WithTimeout deadline, and route the call through the
// resilience.Guard from WithGuard. For streams the guard wraps stream
// establishment only; messages exchanged after establishment are not
// guarded.
//
// Credentials are explicit: New fails closed unless WithInsecure or
// WithTLS is passed. WithTLS loads client cert/key and CA files and fails
// closed on read or parse errors.
//
// Lifecycle: New performs no IO and does not dial; the returned
// *grpc.ClientConn connects lazily and is closed by the caller.
//
// Errors: <package>: <message> prefix; sentinels ErrNoCredentials and
// ErrInsecureAndTLS live in errors.go; branch with errors.Is/errors.As.
//
// Concurrency: safe for concurrent use. No globals, no init wiring; ctx
// is never stored.
package grpcclient
