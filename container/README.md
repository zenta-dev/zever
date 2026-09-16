# container

`container` wires every configured service into one lazily resolved instance
per process, built once from a `config.Config`.

```go
import (
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

c := container.New(config.Default())
db, err := c.DB()
```

There is no global state and no `init()` wiring. A server and a worker
process each build their own `Container` from the same config. `New(nil)`
is valid: an untouched container closes cleanly without opening anything.

## Accessors

Each service resolves on first use and caches the result. On error the
cached state clears so the next call retries.

| Service | Accessor | Notes |
|---|---|---|
| ai | `AI()` | |
| analytics | `Analytics()` | |
| auth | `Auth()` | |
| billing | `Billing()` | |
| cache | `Cache()` | leaf dependency, closed last |
| crypto | `Crypto()` | |
| db | `DB()` | plus `Transactor()`, `CachedDB()` helpers |
| document | `Document()` | |
| eventbus | `Eventbus()` | |
| flag | `Flag()` | |
| geo | `Geo()` | |
| grpc | `GRPC(opts...)` | lazy `*grpc.Server` singleton, no registry |
| i18n | `I18n()` | |
| idempotency | `Idempotency()` | |
| job | `Job()` | `*job.Dispatcher` over resolved `Queue` |
| log | `Log()` | |
| mailer | `Mailer()` | |
| media | `Media()` | |
| notification | `Notification()` | |
| observability | `Observability()` | |
| password | `Password()` | |
| payment | `Payment()` | |
| permission | `Permission()` | |
| queue | `Queue()` | leaf dependency, closed last |
| ratelimit | `Ratelimit()` | |
| router | `Router()` | |
| scheduler | `Scheduler()` | shares `Cache`/`Queue`, closed first |
| search | `Search()` | |
| session | `Session()` | |
| storage | `Storage()` | |
| tenant | `Tenant()` | |
| vectorstore | `VectorStore()` | |
| webhook | `Webhook()` | |
| workflow | `Workflow()` | |

## Job / Scheduler wiring

`Job()` has no registry of its own. It builds a `*job.Dispatcher` over the
already-resolved `Queue`:

```go
q, _ := c.Queue()
_ = q
d, err := c.Job() // d.Q is the shared queue instance
```

`Scheduler()` injects the already-resolved `Cache` and `Queue` into the
scheduler options so the scheduler shares connections instead of opening
redundant ones. Resolving `Scheduler()` therefore resolves `Cache` and
`Queue` as a side effect. `Job()` resolves `Queue` the same way.

## Close order

`Close(ctx)` closes only services already resolved. It never opens anything:
untouched services stay untouched, so a failing factory on an unused
service cannot fail shutdown.

```
scheduler, job        dependents holding cache/queue refs, first
snapshots             everything without explicit ordering
cache, queue          leaf dependencies, last
grpcServer            independent GracefulStop/Stop, bounded by ctx
```

Details:

- Per-service bound: 5s timeout applies only when the parent context
  carries no deadline; otherwise the parent deadline bounds each wait.
- Shutdown shape probing (`closeAny`): `Close(context.Context) error`,
  then `Close() error`, then `Stop() error` (the scheduler declares
  `Stop() error` instead of `Close`), then no-op nil (for example
  `*job.Dispatcher`, which has no shutdown method).
- Shared instances close once: pointer-identity dedup keeps a `keepAlive`
  set for the duration of `Close` so an address cannot be reclaimed and
  reused mid-shutdown.
- Failures join: `Close` returns `errors.Join` of per-service errors.
  Panics become `*ClosePanicError`, timeouts become `*CloseTimeoutError`.
  Error values name the service only, never resolved values.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := c.Close(ctx); err != nil {
	var te *container.CloseTimeoutError
	var pe *container.ClosePanicError
	_ = te
	_ = pe
}
```

## Leak tests

Every `go test ./container/` run ends with `goleak.VerifyTestMain`: no
goroutine may outlive the package tests. Resolve the in-memory subset and
`Close` it; per-service timeout goroutines always observe their deadline
and exit. Keep timeout-test blocks short (parent deadline ~100ms, block
~500ms) and let the straggler finish before the test returns.
