# container

`container` wires every configured service into one lazily resolved instance
per process. Build it once with explicit typed providers, or fall back to the
config-based accessors for a quick start.

Service configuration comes only from `.env`, `.env.local` and the process
environment. A config file left over from an older release is not read at all:
the framework never looks for it and never warns about it.

```go
import (
	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
)

// One Ref per service instance, one Provide per adapter.
var DB = db.DefaultRef()

func Providers() []container.Provider {
	return []container.Provider{container.Provide(DB, sqlite.New)}
}

c := container.New(nil, Providers()...)
db, err := c.DB()
```

There is no global state and no `init()` wiring. A server and a worker
process each build their own `Container` from the same providers. `New(nil)`
is valid: an untouched container closes cleanly without opening anything.

## Typed wiring (Ref and Provide)

The container never resolves an adapter by name on its own. A `ref.Ref[T, O]`
names one wired instance by `Service` (the config service key) and `Name`
(`"default"` unless renamed); `container.Provide` binds it to a factory.

```go
import (
	"github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
)

var DB = db.DefaultRef()
var Cache = cache.DefaultRef()

func Providers() []container.Provider {
	return []container.Provider{
		container.Provide(DB, sqlite.New),
		container.Provide(Cache, memory.New),
	}
}
```

`core/<battery>` exposes `Ref(name)` and `DefaultRef()` for each service.
`container.Get(c, ref)` resolves any wired ref; the accessors (`c.DB()`,
`c.Cache()`) resolve the default instance when a provider exists and
otherwise fall back to the config-based path.

Dependency forms: `Provide`, `Provide1`, `Provide2`, and `Provide3` take
typed dependencies in order. `container.Option` values customize one call:
`Lazy` (default), `Eager` (build during `New`), `Optional` (a missing
dependency resolves to its zero value), and `WithOptions` (run a typed
applier after the env overlay).

Options for a provider start from `config.DefaultOptions(service, name)`,
then `config.ApplyEnv` overlays the process environment, then `WithOptions`
appliers run. Explicit options win over env. A named instance reads
`SERVICE__NAME__*` env keys, not `SERVICE_*`.

The process environment is read once, on the first provider build: a
container built without providers never reads it. A config file left over
from an older release is inert — the environment is read from `.env`,
`.env.local` and the process environment only, and the leftover file is
neither opened nor reported. Translate its stanzas into `.env` and delete
the file; the migration guide in `config/README.md` shows the mapping.

Graph validation runs at `New`: duplicate refs, missing non-`Optional`
dependencies, and dependency cycles are recorded per ref and surfaced by
`Get`. The errors are `DuplicateRefError`, `NotWiredError`, and
`DependencyCycleError` (wrapping `ErrDuplicateRef`, `ErrNotWired`,
`ErrDependencyCycle`). `c.Wired()` lists the wired refs as `service[/name]`
labels in sorted order. `zever explain` prints the same graph with drivers,
dependencies, and env keys.

## Config-based path

A container built with a config and no providers takes this path: every
accessor resolves its service from the config and from the adapter registry,
instead of from a wired provider.

Adapters also register caller-side through each battery's core registry
(`adapters/<battery>/<name>`). The config-based accessors use that registry
when no provider covers the default ref:

```go
import (
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
)

authjwt.Register()
dbsqlite.Register()

c := container.New(config.Default())
```

Resolving an unregistered adapter fails with `UnknownAdapterError`
naming the missing `Register` call. `cmd/zever` registers every adapter
at startup, so CLI behavior is unchanged. Prefer `Provide` for new code.

## Accessors

Each service resolves on first use and caches the result. On error the
cached state clears so the next call retries.

Concurrent first calls share one build. Callers waiting on a build that fails
(or panics, surfacing as `BuildPanicError`) all receive that build's error
without re-running the builder; only a call made after the build settled
retries.

Builders run on whichever goroutine resolves the service first and do not
inherit that caller's context: accessors take no `ctx`, so a canceled first
request cannot poison a process-wide singleton. Long-lived work a build starts
uses the container's root context, which is detached from callers (no
cancellation, deadline, or values) and canceled by `Close`.

37 rows below are the configured services (same set as `config/README.md`
and `knownServiceNames()` in `config/config.go`); `agent`, `grpc`,
`grpcclient`, `job`, and `rag` resolve over other services and take no
`config` section.

| Service | Accessor | Notes |
|---|---|---|
| agent | `Agent()` | `*agent.Loop` over resolved `AI`, closed first |
| ai | `AI()` | |
| analytics | `Analytics()` | |
| auth | `Auth()` | |
| billing | `Billing()` | |
| cache | `Cache()` | leaf dependency, closed last |
| cdn | `CDN()` | |
| crypto | `Crypto()` | |
| db | `DB()` | plus `Transactor()` helper |
| document | `Document()` | |
| eventbus | `EventBus()` (`Eventbus()` alias) | |
| flag | `Flag()` | |
| geo | `Geo()` | |
| grpc | `GRPC(opts...)` | lazy `*grpc.Server` singleton, no registry |
| grpc client | `GRPCClient(target, opts...)` | returns a `*grpc.ClientConn` for the target |
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
| resilience | `Resilience()` | `resilience.Manager` |
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

```sh
SEARCH_ADAPTER=db
SEARCH_DSN="postgres://app:secret@db:5432/app?sslmode=require"
SEARCH_DEDICATEDPOOL=false   # default false shares; true restores a private pool
```

- Share-by-default: two identical DSNs are unambiguous operator intent, so
  existing files keep working unchanged — sharing activates only on
  exact-DSN match (or sqlite same-file match).
- Single cap: the shared pool is built with the **first opener's** pool
  knobs; later borrowers' pool knobs are ignored (their table names,
  prefixes, TTLs still apply). A nonzero `MaxConns` differing from the
  pool's warns on stderr naming the service, never the DSN. Every
  mismatch is also recorded: `Container.PoolMismatches()` returns them in
  borrow order and `Container.PoolStatus()` describes each shared pool
  (backend, borrowers, effective cap) with no DSN content. `zever doctor`
  prints both as `pool:` warnings (plus `pool_warnings` in `--json`)
  with the `dedicated_pool` fix hint; warnings never fail the command.
- Borrowers keep `owns=false`: their `Close` stays a no-op and the
  registry closes each pool once, after every borrower.
- Observability follows the redaction rule: log lines carry only the
  parsed postgres host/dbname (never userinfo, query, or file paths), and
  errors are prefixed `container: <service> (shared pool): ...`.
- Escape hatch: queue, workflow, and scheduler under sustained load should
  run with `dedicated_pool: true` and, for full isolation, a **separate
  database** (distinct DSN), per the Solid Queue guidance. Rule of thumb:
  share the pool while the three are idle-to-light (one pool, one cap to
  tune); split to `dedicated_pool: true` when any one of them sustains
  backlog — a long-polling queue worker, a workflow with parallel fan-out,
  or a scheduler with sub-minute cadences can starve the others under a
  single cap, and pool-wait latency shows up as queue `Pop`/`Ack`
  slowness, not as an error. A `zever doctor` pool warning naming one of
  these three is the signal to split that service first.
- WAL stays opt-in for shared sqlite files
  (`?_pragma=journal_mode(WAL)`); sharing does not change journal mode.

## Close order

`Close(ctx)` closes only services already resolved. It never opens anything:
untouched services stay untouched, so a failing factory on an unused
service cannot fail shutdown.

```
provided instances    wired refs, reverse dependency order, first
scheduler, job        dependents holding a queue ref
snapshots             everything without explicit ordering
cache, queue          leaf dependencies, last
db, registry pools    shared pools, after every borrower (no use-after-close)
grpcServer            independent GracefulStop/Stop, bounded by ctx
```

Provided instances close in reverse dependency order so dependents shut
down before the pools and services they borrowed. Only resolved providers
close; `Lazy` providers that were never requested stay untouched.

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
  Panics become `ClosePanicError`, timeouts become `CloseTimeoutError` (value
  types — `errors.As` against a pointer target never matches).
  Error values name the service only, never resolved values.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := c.Close(ctx); err != nil {
	var te container.CloseTimeoutError
	var pe container.ClosePanicError
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
