# middleware

Plain HTTP middleware and gRPC unary interceptors built over resolved
`log`, `ratelimit`, and `observability` instances. No registry, no adapters:
call the constructors once when wiring a server.

## HTTP

```go
handler := middleware.RequestLogger(logger)(
    middleware.Tracing(provider)(
        middleware.RateLimit(limiter, middleware.RemoteAddrKey)(
            middleware.Recover(logger)(mux))))
```

Order matters: `RequestLogger` outermost so a recovered panic is logged with
its 500 status; `Recover` innermost (closest to the handler) so it catches
panics from inner middleware too.

| Constructor | Behavior |
|---|---|
| `RequestLogger(logger)` | One structured line per request: method, path, status, `duration_ms`, plus `request_id` when present. |
| `Recover(logger)` | Recovers handler panics, logs value + stack, responds 500 `{"error":"internal error"}`. If the handler already committed the response, logs only (no rewrite). |
| `RateLimit(limiter, keyFunc)` | Denies with 429 JSON + `Retry-After` (ceiled seconds). Limiter errors fail open. Default key: `RemoteAddrKey` (client IP). |
| `Tracing(provider)` | Span per request named by path, `http.status_code` attribute, span error on 5xx, `http.request` counter tagged method/status. |

The status recorder reuses a single wrapper when middleware chain (no
double-wrap) and exposes `Unwrap` so `http.ResponseController` still reaches
`Flush`/`Hijack` on the underlying writer.

## gRPC

Pair each HTTP middleware with its unary interceptor: `RecoverUnaryServerInterceptor`
(panic → `Internal`), `RateLimitUnaryServerInterceptor` (deny →
`ResourceExhausted`, errors fail open, key via `PeerAddrKey`), and
`TracingUnaryServerInterceptor` (span per `FullMethod`, `rpc.request`
counter tagged method/outcome). Recovery interceptors go last in the chain
so other interceptors observe recovered state.
