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

## Adapters

The container wires no adapters itself. Every adapter registers caller-side
via its own module's `Register()` — one call, no I/O, idempotent. Each
adapter lives in its own module under `adapters/<battery>/<name>`:

```go
import (
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

authjwt.Register()
dbsqlite.Register()
```

Resolving an unregistered adapter fails with `UnknownAdapterError`
naming the missing `Register` call. `cmd/zever` registers every adapter
at startup, so CLI behavior is unchanged. Generated apps register only
the adapters they selected.

## Accessors

Each service resolves on first use and caches the result. On error the
cached state clears so the next call retries.

| Service | Accessor | Notes |
|---|---|---|
| agent | `Agent()` | `*agent.Loop` over resolved `AI`, closed first |
| ai | `AI()` | |
| analytics | `Analytics()` | |
| auth | `Auth()` | |
| billing | `Billing()` | |
| cache | `Cache()` | leaf dependency, closed last |
| crypto | `Crypto()` | |
| db | `DB()` | plus `Transactor()` helper |
| document | `Document()` | |
| eventbus | `EventBus()` (`Eventbus()` alias) | |
| flag | `Flag()` | |
| geo | `Geo()` | |
| grpc | `GRPC(opts...)` | lazy `*grpc.Server` singleton, no registry |
| i18n | `I18n()` | |
| idempotency | `Idempotency()` | |
| job | `Job()` | `*job.Dispatcher` over resolved `Queue` |
| lock | `Lock()` | |
| log | `Log()` | |
| mailer | `Mailer()` | |
| media | `Media()` | |
| notification | `Notification()` | |
| observability | `Observability()` | |
| outbox | `Outbox()` plus `OutboxRelay(ctx)` helper | relay attaches the transport-selected publisher and starts |
| password | `Password()` | |
| payment | `Payment()` | |
| permission | `Permission()` | |
| queue | `Queue()` | leaf dependency, closed last |
| rag | `RAG()` | `*rag.Engine` over resolved `AI` + `VectorStore`, closed first |
| ratelimit | `RateLimit()` (`Ratelimit()` alias) | |
| router | `Router()` | |
| scheduler | `Scheduler()` | shares `Queue` (via `Job()`), closed first |
| search | `Search()` | |
| secrets | `Secrets()` | |
| session | `Session()` | |
| storage | `Storage()` | |
| tenant | `Tenant()` | |
| vectorstore | `VectorStore()` | |
| webhook | `Webhook()` | |
| workflow | `Workflow()` | |

## Readiness

`Ready(ctx)` is the single readiness aggregate: `nil` means the process should
receive traffic, otherwise the reasons are joined into one error. The
scaffolded server's `/readyz` handler and the gRPC health status both read it,
so every transport answers from one decision.

```go
if err := c.Ready(probeCtx); err != nil {
	w.WriteHeader(http.StatusServiceUnavailable) // errors.Is(err, container.ErrDatabaseUnavailable)
}
```

- **db**: always checked — it must resolve and answer a ping. Bounding the
  probe is the caller's job (the scaffold uses a 3s timeout).
- **outbox**: checked only when `outbox.stall_readiness` is set, and then only
  while `Status().Stalled` is true, so a relay whose messages are backing up
  drains traffic instead of silently queueing work.
- The outbox check is best-effort: a relay that fails to resolve is skipped,
  never a readiness failure. Turning the flag on can therefore only ever make
  readiness stricter for a relay that actually exists.

Sentinels: `ErrDatabaseUnavailable`, `ErrRelayStalled`, both carrying typed
`DatabaseUnavailableError` / `RelayStalledError` for classification.

## Outbox relay wiring

`Outbox()` resolves the store but does not start the relay: `core/outbox`
carries only the informational selector string (`outbox.publisher`), never a
live transport. `OutboxRelay(ctx)` is the wiring that joins the two — resolve
the store, resolve the selected transport, attach it through
`outbox.PublisherSetter`, then `Start`:

```go
store, err := c.OutboxRelay(ctx) // relay is draining; Record into the same store
```

- **Selector**: `outbox.publisher: eventbus` publishes through the resolved
  `EventBus()`; `queue` and the empty default publish through the resolved
  `Queue()`. The empty default prefers the queue, since a queued message
  survives a subscriber restart. Anything else is rejected by
  `outbox.Options.Validate` during resolution.
- **One instance**: the returned store is the same cached one `Outbox()`
  resolves and `Close` shuts down, so Record keeps writing to the table the
  relay drains. Like `Job()`, starting the relay resolves its transport as a
  side effect.
- **Fail loud**: an unresolvable selected transport, a store that does not
  implement `outbox.PublisherSetter`, or a `Start` failure is returned as an
  error. A relay that claims messages and drops them looks healthy until the
  events are gone, so there is no silent no-op path.
- **Replicas are safe**: claims are atomic inside the adapter (one statement
  takes the rows and their lease; postgres uses `FOR UPDATE SKIP LOCKED`), so
  several replicas can poll the same table. A replica that dies mid-publish
  releases its rows after `outbox.lock_seconds`.
- `queue.Queue` and `eventbus.EventBus` have no routing-key argument, so
  `Message.Key` travels as the `outbox-key` header
  (`shared/outboxbridge`); read it back with `outboxbridge.Key(headers)`.

## Job / Scheduler wiring

`Job()` has no registry of its own. It builds a `*job.Dispatcher` over the
already-resolved `Queue`:

```go
q, _ := c.Queue()
_ = q
d, err := c.Job() // d.Q is the shared queue instance
```

`Scheduler()` injects the already-resolved job dispatcher (via `Job()`)
into the scheduler options, so the scheduler shares the same `Queue`
connection instead of opening a redundant one. Resolving `Scheduler()`
therefore resolves `Job()` and, transitively, `Queue` as a side effect.

## Agent / RAG wiring

`Agent()` and `RAG()` follow the `Job()` pattern: no adapter registry and no
`config` entry. They compose already-resolved backends:

```go
loop, _ := c.Agent() // *agent.Loop over the shared AI instance
engine, _ := c.RAG() // *rag.Engine over the shared AI + VectorStore instances
```

Both accessors take optional functional options, applied only on first
resolution:

```go
loop, _ := c.Agent(container.WithMaxParallel(4))
engine, _ := c.RAG(container.WithHybridSearch(), container.WithTopK(10))
```

The model name comes from the `ai` options. Both hold references to
snapshot-closed backends, so `Close` shuts them first (with `scheduler` and
`job`), before any backend they wrap.

## Shared pools

The container resolves **one pool per DSN** and shares it across every
db-backed battery pointing at the same DSN (`db`, `cache`, `queue`,
`search`, `session`, `idempotency`, `workflow`, `scheduler`,
`vectorstore`), instead of opening one pool per battery. Matching is the
exact DSN string after trim: a postgres URL with and without a query
string are different pools. SQLite file paths share after
`filepath.Clean` (relative paths resolve against the process CWD, so every
battery must run with the same CWD); `:memory:` (including
`file::memory:?cache=shared`) never shares.

```yaml
search:
  adapter: db
  options:
    dsn: "postgres://app:secret@db:5432/app?sslmode=require"
    dedicated_pool: false   # default false shares; true restores a private pool
```

- Share-by-default: two identical DSNs are unambiguous operator intent, so
  existing files keep working unchanged — sharing activates only on
  exact-DSN match (or sqlite same-file match).
- Single cap: the shared pool is built with the **first opener's** pool
  knobs; later borrowers' pool knobs are ignored (their table names,
  prefixes, TTLs still apply). A nonzero `MaxConns` differing from the
  pool's warns on stderr naming the service, never the DSN.
- Borrowers keep `owns=false`: their `Close` stays a no-op and the
  registry closes each pool once, after every borrower.
- Observability follows the redaction rule: log lines carry only the
  parsed postgres host/dbname (never userinfo, query, or file paths), and
  errors are prefixed `container: <service> (shared pool): ...`.
- Escape hatch: queue, workflow, and scheduler under sustained load should
  run with `dedicated_pool: true` and, for full isolation, a **separate
  database** (distinct DSN), per the Solid Queue guidance.
- WAL stays opt-in for shared sqlite files
  (`?_pragma=journal_mode(WAL)`); sharing does not change journal mode.

## Close order

`Close(ctx)` closes only services already resolved. It never opens anything:
untouched services stay untouched, so a failing factory on an unused
service cannot fail shutdown.

```
scheduler, job        dependents holding a queue ref, first
snapshots             everything without explicit ordering
cache, queue          leaf dependencies, last
db, registry pools    shared pools, after every borrower (no use-after-close)
grpcServer            independent GracefulStop/Stop, bounded by ctx
```

Details:

- Per-service bound: 5s timeout applies only when the parent context
  carries no deadline; otherwise the parent deadline bounds each wait.
- Shutdown shape probing (`closeAny`): `Close(context.Context) error`,
  then `Close() error`, then `Stop() error` (the scheduler declares
  `Stop() error` instead of `Close`), then `Shutdown(context.Context) error`
  (observability providers declare `Shutdown` so buffered spans/metrics
   flush), then no-op nil (for example `*job.Dispatcher`, which has no
   shutdown method). Crypto, log, password, permission, and router declare no
   Close/Stop/Shutdown and probe as no-op nil.
- Close-shape standard: pooled or network-backed top-level services that
  may block on shutdown take `Close(ctx)` (`db`, `cache`, `storage`,
  `lock`, `container` itself); lightweight handles, iterators, statements,
  and instant-close in-memory adapters take `Close()` (`auth`, `queue`,
  `session`, `payment`, `eventbus.Pusher`, `ratelimit.Limiter`, `db.Rows`,
  `db.Stmt`); the scheduler alone uses `Stop()` for its cron lifecycle.
  `closeAny` probes in that order so callers never guess.
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
~500ms) and let the straggler finish before the test returns. Sanctioned
exception: timeout-path tests using `ignoreCtx` stubs sleep past the
stub's block in teardown only (never for sync) so goleak stays clean.

## Outbox relay tests

`OutboxRelay` tests seed the container's lazy slots with stub services
(`mustSeed`, the same seam `TestTransactor_unsupported` uses), so they open no
database, broker, or timer: a stub store publishes one message from `Start`,
and a stub queue or bus records it. They close with a live context
(`closeRelayContainer`) rather than `closeContainer`, whose `t.Context()` is
already canceled by the time cleanups run and would report a phantom
"close timed out".
