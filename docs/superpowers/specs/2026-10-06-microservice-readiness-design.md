# Microservice Readiness Layer — Design Spec

**Date:** 2026-10-06
**Status:** Approved (design), implementation in progress
**Owner:** @erhahahaa

## Problem

zever scaffolds production services (gRPC+HTTP, health/ready, graceful shutdown,
OTLP tracing, redis/postgres-backed lock/queue/eventbus/idempotency/workflow) but
lacks four building blocks that every microservices platform needs and that a
monolith can also use:

1. **Resilience** — no circuit breaker, bulkhead, or policy composition.
   `shared/retry` is backoff only; `core/ratelimit` is token-bucket only.
2. **Transactional outbox/inbox** — `core/eventbus`/`core/queue` publish directly
   (dual-write hazard: business write commits, publish fails → lost event).
3. **gRPC client-side load balancing + discovery** — server-side only; no
   `container.GRPCClient`, no `round_robin`, no retry policy, no resolver helper.
4. **Saga/compensation** — `core/workflow` runs durable steps but has no
   compensation, pivot, or reverse rollback.

## Principles

- **Monolith → modular monolith → microservices.** Every phase is useful without
  network. Resilience protects in-process calls; outbox decouples in-process
  modules; only LB/discovery/saga require distribution.
- **Follow existing battery pattern.** `core/<b>` interface + `registry`
  (`Register`/`Open`), `adapters/<b>/<a>` one Go module each, `container` sole
  resolver, zero-infra `config.Default()`, conformance kit `core/<b>/<b>test/`.
- **Non-breaking.** New separate interfaces (mirrors `workflow.StepRegistrar`
  precedent) so no existing implementer breaks. Public config/container changes
  documented in `CHANGELOG.md` under `## [Unreleased]`.
- **Fail closed** on unsupported features; sentinel errors, `<battery>:` prefix,
  `errors.Is`/`errors.As` compatible; no globals/`init()`; `ctx` first arg, never
  stored; deterministic tests (no sleep/network/unseeded rand).

## Phase A — `core/resilience`

Process-local resilience policy composition. Default adapter `memory`.

```go
type Adapter string // memory (default), redis (distributed state, opt-in)
type State string    // closed, open, half-open

type Guard interface {
    Execute(ctx context.Context, fn func(context.Context) error) error
    State() State
    Name() string
    Close() error
}

type Manager interface {
    Guard(name string) (Guard, error) // named per-dependency guard, lazily created
    Close() error
}

type Factory func(opts Options) (Manager, error)
func Register(adapter Adapter, f Factory) error
func Open(adapter Adapter, opts Options) (Manager, error)
func Do[T any](ctx context.Context, g Guard, fn func(context.Context) (T, error)) (T, error)
```

**Composition order (outermost → innermost):**
`Timeout → Retry → Breaker → Bulkhead → fn`.

- Timeout governs the whole operation including retries.
- Retry reuses `shared/retry.Policy` (backoff + jitter).
- Breaker: `sony/gobreaker/v2` rolling window; `IsExcluded` returns true for
  `context.Canceled`/`context.DeadlineExceeded` so caller cancellation never
  trips the breaker; `IsSuccessful` classifies business errors as success when
  configured.
- Bulkhead: `golang.org/x/sync/semaphore` weighted, bounded wait, fail-fast.

**Hooks:** `OnStateChange(name, from, to State)` emits `core/observability`
counter + `log` line. Optional `Provider observability.Provider` in Options.

**Adapters:** `adapters/resilience/inproc` (required, gobreaker + x/sync);
`adapters/resilience/redis` (stretch, `gobreaker/v2` `DistributedCircuitBreaker`
+ `gobreaker/v2/redis` store, sharing via `shared/redisclient`).

**Options:** `Timeout`, `Retry` (`shared/retry.Policy`), `Breaker{Enabled,
MaxRequests, Interval, BucketPeriod, Timeout, MinRequests, FailureRatio,
ConsecutiveFailures}`, `Bulkhead{MaxConcurrent, MaxQueue, MaxWait}`, `Provider`,
`Name`.

**Errors:** `ErrOpenState`, `ErrTooManyRequests`, `ErrBulkheadFull`, `ErrTimeout`,
`ErrNilFactory`, `InvalidAdapterError`, `UnknownAdapterError`, `DuplicateError`,
`InvalidOptionsError`.

## Phase B — `core/outbox`

Transactional outbox + inbox. Fixes dual-write by writing the event in the same
`db.Tx` as the business row; a relay publishes later with at-least-once delivery.

```go
type Adapter string // memory (dev), db (default, sqlite/postgres polling), cdc (postgres logical replication)

type Message struct {
    ID string; Topic, Key string; Payload []byte
    Headers map[string]string; CreatedAt time.Time; Attempts int
}
type Store interface {
    Record(ctx context.Context, tx db.Tx, msg Message) error // SAME tx as business write
    Start(ctx context.Context) error
    Status() Status
    Close() error
    Name() string
}
type Status struct { Pending, Processed, Failed int64; LastError string; Stalled bool }
type Publisher interface { Publish(ctx context.Context, msg Message) error }
type Inbox interface {
    Process(ctx context.Context, tx db.Tx, eventID string, fn func(context.Context, db.Tx) error) error
}
```

**Adapters:**
- `adapters/outbox/db` (default): polling relay with
  `SELECT ... FOR UPDATE SKIP LOCKED`, claim+mark atomic, retry via
  `shared/retry`, DLQ after `MaxAttempts`, retention cleanup, `traceprop.Inject`
  on publish, `Publisher` bridges `eventbus`/`queue`. `RegisterShared` for pool
  sharing. Implements `Inbox` with unique `event_id` in same tx.
- `adapters/outbox/cdc`: postgres logical replication (`jackc/pglogrepl`),
  `pg_logical_emit_message(transactional=true, prefix, content)` producer +
  `START_REPLICATION ... (proto_version '1', publication_names '...', messages
  'true')` consumer; same `Message`/`Store` API. Live test gated by
  `POSTGRES_DSN`; requires `wal_level=logical`.
- `adapters/outbox/memory`: in-process immediate publish, dev/test only.

**Options:** `DSN`, `Table`, `Publisher` (`eventbus`|`queue`), `PollInterval`,
`BatchSize`, `MaxAttempts`, `Retry` (`shared/retry.Policy`), `Retention`,
`LockSeconds`, `NotifyChannel`, `Prefix`, `Slot`, `Publication`, `DedicatedPool`.

**Default:** `db` (sqlite path via `shared/dbconn.SplitDSN`) — zero-infra *and*
durable.

## Phase C — `shared/grpcclient` + `container.GRPCClient`

No new battery. A helper constructing client connections with LB, retry,
tracing, resilience.

```go
func New(ctx context.Context, target string, opts ...Option) (*grpc.ClientConn, error)
```

- Resolvers: `static://` (`StaticResolver`, register under `static`) + builtin
  `dns:///host:port`.
- `grpc.WithDefaultServiceConfig` = `{"loadBalancingConfig":[{"round_robin":{}}]}`,
  merged with optional per-method `retryPolicy` (`maxAttempts` ≤5, backoff,
  `retryableStatusCodes`) and `retryThrottling`.
- Unary + stream interceptors: `traceprop` inject, Phase-A `resilience.Guard`
  (breaker+timeout), metadata propagation (tenant/auth), logging.
- TLS credentials option; `WithInsecure()` dev default.
- `container.GRPCClient(target string, opts ...grpcclient.Option)
  (*grpc.ClientConn, error)` lazy-cached per target+key; closes with container.

## Phase D — Saga on `core/workflow`

Extend, non-breaking (separate interfaces).

```go
type SagaStep struct {
    Name       string
    Execute    StepFunc
    Compensate StepFunc
    Pivot      bool // after pivot: retry forward, never compensate
}
type SagaRegistrar interface { RegisterSaga(name string, steps []SagaStep) }
type SagaRunner    interface { RunSaga(ctx context.Context, name string, input any, workflowID string) (RunID, error) }
type SagaStatus struct { RunID RunID; Name string; CurrentStep int; Status string; FailedStep int; Err string }
```

- Durable per-step progress (postgres tables in `adapters/workflow/db`).
- Forward execution; on failure compensate executed steps in reverse order.
- Idempotency key = `runID + ":" + stepIndex`, reusing `core/idempotency`.
- Compensation retry + DLQ + `compensation_failures` table.
- Stuck-run sweeper reuses existing lease recovery.
- Per-step observability spans.
- `adapters/workflow/memory` implements saga in-process (non-durable).
- **D.2 (deferred):** `.zen` `saga {}` declaration → generated registration.

## Cross-cutting (definition of done per battery)

For each new `core/<b>` + adapter module:
- `core/<b>/`: `doc.go`, `adapter.go`, `errors.go`, `options.go`, `<b>.go`,
  `shared.go` (if needed), `<b>test/` conformance kit, tests
  (`cover`/`edge`/`bench`/`example`).
- `adapters/<b>/<a>/`: own `go.mod`, `doc.go`, `options.go`, `register.go`
  (`Register()` + `RegisterShared` when DB-backed), implementation, tests
  (live gated by `POSTGRES_DSN`).
- Wiring: root `go.work`; `config/{config,merge,validate,env}.go` +
  `config/README.md` service table; `container/{container,services}.go` +
  `container/services_test.go`; `docs/.../reference/adapters-matrix.mdx` +
  `api-index.mdx`; `CHANGELOG.md` `## [Unreleased]`.
- `make deps-sync` after any dependency add (`gobreaker/v2`, `x/sync`,
  `pglogrepl`). Pass `make check`.

## Execution model (parallel, non-blocking)

- **Coordinator (main thread)** owns shared files (`go.work`, `config/*`,
  `container/*`, docs index, `CHANGELOG.md`, `Makefile`, `tools/pr-runner.sh`)
  and integration commits. Never edits module internals.
- **Workers (subagents in `.worktrees/<wave>-<name>`)** each own a disjoint
  module dir only. Each runs `tools/pr-runner.sh`: branch → commit → push →
  `gh pr create` → `gh pr checks --watch --fail-fast` → green:
  `gh pr merge --auto --squash`; red: read logs, fix, push, loop ≤5. Subagent
  watches CI, so the main thread is never blocked.
- **Backstop:** existing `.github/workflows/automerge.yml` squash-merges owner
  PRs on `ci-gate` green.
- **Waves:** W1 = resilience + outbox/db (disjoint); W2 = grpcclient +
  outbox/cdc + integration wiring of W1; W3 = saga + integration; W4 = D.2.

## Risks

- Shared-file merge conflicts → coordinator ownership + waves.
- New deps → `make deps-sync` across all modules + standalone examples.
- Public config/container API → pre-1.0, `CHANGELOG.md`, maintainer review.
- Live tests need `POSTGRES_DSN`; CI per-module runs with `GOWORK=off`.
- `automerge.yml` only acts on author `erhahahaa` (confirmed authenticated).

## References

- gobreaker v2.4.0 + distributed/redis store — github.com/sony/gobreaker
- gRPC retry & service config — grpc.io/docs/guides/retry, /service-config
- Transactional outbox (polling + CDC) — microservices.io, decodable.co
- pglogrepl + `pg_logical_emit_message` — github.com/jackc/pglogrepl
- Saga orchestration/pivot/compensation — microservices.io/patterns/data/saga
