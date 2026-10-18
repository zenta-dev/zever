# serve

Shared boot, mount, and shutdown runtime for every Zever-generated server
binary. `serve` resolves config, builds the container, mounts modules on the
resolved router, serves HTTP (and opt-in gRPC), answers health probes, and
shuts down gracefully on SIGINT or SIGTERM.

`serve` is adapter-agnostic: it imports `core` batteries and `shared` leaves
only, never an `adapters/*` module. Adapter registration stays in the
generated app package, which passes providers into `Main` and `Worker`.

## Entry points

| Call | Behavior |
|---|---|
| `Run(providers, modules, opts...)` | Boot the server and block until SIGINT/SIGTERM. |
| `Main(providers, modules, opts...)` | `Run` plus the `doctor` dispatch. |
| `Worker(providers, opts...)` | Boot a queue/scheduler worker; no HTTP listener. |
| `Doctor(providers, opts...)` | Resolve every boot battery and print one JSON report. |

None of them panic, call `log.Fatal`, or call `os.Exit`. The caller prints the
returned error to stderr and exits non-zero. `Main` and `Doctor` dispatch on
the `doctor` or `--doctor` argument.

## Modules

A `serve.Module` is one mountable unit:

```go
type Module interface {
	Name() string
	Mount(r router.Router, c *container.Container) error
}
```

`Mount` runs once at boot, after the shared gRPC server (when enabled) is
built. A module that serves gRPC calls `c.GRPC()` inside `Mount` to register
its services. Modules that declare gRPC methods also implement `GRPCModule`:

```go
type GRPCModule interface {
	Module
	GRPCPolicies() map[string]authz.Policy
}
```

`serve` merges every module's policies into one fail-closed interceptor
before the server is built. A method without a policy entry is denied. A
module declaring policies without a `GRPC_ADDR` fails the boot
(`ErrGRPCPoliciesWithoutAddr`). Duplicate module names fail closed
(`ErrDuplicateModule`).

## Options and environment

Explicit options win over the process environment, which wins over defaults.

| Option | Env | Default | Meaning |
|---|---|---|---|
| `WithPort(port)` | `PORT` | `8080` | HTTP listen port. |
| `WithGRPCAddr(addr)` | `GRPC_ADDR` | empty | gRPC listen address; empty disables gRPC. |
| `WithPprofAddr(addr)` | `PPROF_ADDR` | empty | Loopback pprof address; empty disables profiling. |
| `WithEnv(env)` | | | Replace the env `serve` resolves config from (tests). |

`WithEnvOverride` is an alias of `WithEnv` kept for pre-release call sites.
`ZEVER_ENV` marks a non-production environment (`dev`, `test`, `development`),
which permits the deterministic dev crypto key and enables gRPC reflection.
`GRPC_REFLECTION=true` opts into reflection explicitly; an explicit `false`
wins even in dev.

## Security defaults

- **TLS 1.2 floor.** `ROUTER_TLS_CERT` and `ROUTER_TLS_KEY` enable TLS on both
  listeners; `ROUTER_TLS_CLIENT_CA` turns on mutual TLS. The pair must be set
  together. Serving plaintext on a non-loopback address logs one warning per
  listener.
- **Reflection off by default.** Enabled only by `GRPC_REFLECTION=true` or a
  dev `ZEVER_ENV`.
- **Fail-closed authz.** The gRPC interceptor denies any method without a
  policy; the standard health check is allowlisted.
- **Dev-key refusal.** The deterministic dev crypto key is refused outside a
  dev `ZEVER_ENV` unless the provider wires crypto itself.
- **Request bound.** HTTP bodies are capped at `DefaultMaxRequestBodyBytes`
  (4 MiB).
- **Security headers.** `Recover`, `RequestLogger`, `Tracing`, and
  `SecurityHeaders` middleware run on every request; rate limiting runs when
  the config sets a positive rate.

## Health and shutdown

`GET /healthz` always returns 200. `GET /readyz` returns 200 when the DB ping
(3s timeout) and gRPC health succeed, else 503. On SIGINT/SIGTERM the server
flips gRPC health to `NOT_SERVING`, calls `GracefulStop`, shuts the HTTP server
down, then closes the container on a 10s timeout context.

## Errors

| Error | Meaning |
|---|---|
| `ErrDoctorFailed` | A doctor report had one or more failing batteries. |
| `ErrDuplicateModule` | Two modules share one `Name`. |
| `ErrGRPCPoliciesWithoutAddr` | A module declares gRPC policies with no gRPC address. |

`doctor` prints `{"ok": bool, "batteries": [{"name", "ok", "error"}]}` to
stdout and returns `ErrDoctorFailed` when any battery fails.
