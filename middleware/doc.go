// Package middleware provides plain HTTP middleware and unary-gRPC
// interceptor constructors over already-resolved log, ratelimit, and
// observability instances (no adapter registry, nothing pluggable by name).
//
// Each concern ships as an HTTP + unary-gRPC pair with the same semantics:
// recovery from panics, one structured access line per request, rate
// limiting, and tracing/metrics.
//
// Recommended HTTP order is Logging(Recover(handler)) so panics are logged
// as 500s by the outer logging layer; for gRPC, wire recovery last so it
// converts panics before logging and metrics observe them.
package middleware
