# Microservice Readiness Layer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` (recommended) or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add opt-in resilience, transactional outbox/inbox, gRPC client LB/discovery, and saga compensation to zever so the same codebase scales monolith → modular monolith → microservices.

**Architecture:** Four independent modules following zever's battery pattern. Each is a self-contained `core/<b>` interface + registry plus one or more `adapters/<b>/<a>` Go modules. A coordinator owns shared wiring (`go.work`, `config/`, `container/`, docs, `CHANGELOG.md`). Workers implement disjoint module dirs in git worktrees and ship PRs.

**Tech Stack:** Go 1.27, `sony/gobreaker/v2` v2.4.0, `golang.org/x/sync`, `jackc/pglogrepl`, `google.golang.org/grpc`, existing `shared/retry`, `shared/dbconn`, `shared/redisclient`, `shared/traceprop`, `core/db`, `core/observability`, `core/idempotency`, `core/workflow`.

**Spec:** `docs/superpowers/specs/2026-10-06-microservice-readiness-design.md`

---

## Execution protocol (every worker)

Worker = one subagent, one git worktree, one disjoint module dir.

1. Worktree created by coordinator: `.worktrees/w1-resilience`, etc.
2. Worker implements tasks for its module only. It MUST NOT edit `go.work`,
   `config/*`, `container/*`, `docs/**`, `CHANGELOG.md`, `Makefile`, or
   `tools/pr-runner.sh` — coordinator owns those.
3. Worker runs `make fmt-fix && make vet` for its module and
   `GOWORK=off go test ./...` (per-module) until green.
4. Worker runs `tools/pr-runner.sh <branch> "<commit message>" "<PR title>"
   "<PR body>"` which pushes, opens the PR, watches CI, fixes+re-pushes on red
   (≤5 attempts), and squash-auto-merges on green.
5. Worker reports PR URL + final status to coordinator.

Coordinator: after each wave merges, performs the integration PR (wire modules
into `go.work`, config, container, docs, CHANGELOG), then dispatches the next
wave.

---

# Wave 1 (parallel, disjoint)

## Task W1-A: `core/resilience` battery

**Worker dir:** `.worktrees/w1-resilience`
**Owns:** `core/resilience/**`, `adapters/resilience/inproc/**`
**Files:**
- Create: `core/resilience/go.mod`, `core/resilience/doc.go`,
  `core/resilience/adapter.go`, `core/resilience/errors.go`,
  `core/resilience/options.go`, `core/resilience/resilience.go`,
  `core/resilience/guard.go`, `core/resilience/shared.go`
- Create: `core/resilience/resiliencetest/kit.go` (conformance kit)
- Create tests: `core/resilience/resilience_test.go`,
  `core/resilience/edge_test.go`, `core/resilience/options_test.go`,
  `core/resilience/example_test.go`, `core/resilience/bench_test.go`
- Create: `adapters/resilience/inproc/go.mod`,
  `adapters/resilience/inproc/doc.go`, `options.go`, `register.go`,
  `inproc.go`, `inproc_test.go`, `inproc_edge_test.go`

**Interface contract:** exactly as in spec Phase A. Package `resilience`.

- [x] **Step 1: Failing test for registry + options validation**

```go
// core/resilience/resilience_test.go
package resilience_test

import (
    "errors"
    "testing"

    "github.com/zenta-dev/zever/core/resilience"
)

func TestOpenUnknownAdapter(t *testing.T) {
    _, err := resilience.Open(resilience.Adapter("nope"), resilience.Options{})
    if !errors.Is(err, resilience.ErrUnknownAdapter) {
        t.Fatalf("err = %v, want ErrUnknownAdapter", err)
    }
}
```

Run: `GOWORK=off go test ./core/resilience/ -run TestOpenUnknownAdapter`
Expected: FAIL (package does not exist).

- [x] **Step 2: Implement `adapter.go`, `errors.go`, `options.go`, `resilience.go`**

`adapter.go`: `Adapter` enum (`Memory`, `Redis`) + `ParseAdapter` (non-empty
accepted, empty → `InvalidAdapterError`). `errors.go`: sentinels `ErrNilFactory`,
`ErrOpenState`, `ErrTooManyRequests`, `ErrBulkheadFull`, `ErrTimeout`,
`ErrUnknownAdapter`, `ErrDuplicateAdapter`, `ErrInvalidAdapter`,
`ErrInvalidOptions`; typed errors `UnknownAdapterError`, `DuplicateError`,
`InvalidAdapterError`, `InvalidOptionsError` with `Error()` + `Unwrap()`.
`options.go`: `Options` (fields from spec) with `Validate()` joining all
violations via `errors.Join`; defaults consts `DefaultTimeout`,
`DefaultMaxConcurrent`, `DefaultMaxQueue`, `DefaultMaxWait`. `resilience.go`:
`Manager`/`Guard`/`Factory` interfaces, `State` enum, `Do[T]` generic helper,
`registry.New[Adapter, Factory]`, `Register`, `Open`, `Default()`.

Run: `GOWORK=off go test ./core/resilience/ -run TestOpenUnknownAdapter`
Expected: PASS.

- [x] **Step 3: Failing test for guard behavior (closed→open→half-open)**

```go
// core/resilience/resilience_test.go
func TestGuardTripsAndRecovers(t *testing.T) {
    m, err := resilience.Open(resilience.Memory, resilience.Options{
        Breaker: resilience.BreakerOptions{
            Enabled: true, MinRequests: 2, FailureRatio: 0.5, Timeout: 10 * time.Millisecond,
        },
    })
    if err != nil { t.Fatal(err) }
    defer m.Close()
    g, err := m.Guard("dep")
    if err != nil { t.Fatal(err) }

    fail := func(ctx context.Context) error { return errors.New("boom") }
    for i := 0; i < 2; i++ { _ = g.Execute(context.Background(), fail) }
    if g.State() != resilience.StateOpen {
        t.Fatalf("state = %s, want open", g.State())
    }
    if err := g.Execute(context.Background(), fail); !errors.Is(err, resilience.ErrOpenState) {
        t.Fatalf("err = %v, want ErrOpenState", err)
    }
    time.Sleep(15 * time.Millisecond) // breaker open timeout, not a sync primitive
    if err := g.Execute(context.Background(), func(context.Context) error { return nil }); err != nil {
        t.Fatalf("half-open probe err = %v", err)
    }
}
```

Run: `GOWORK=off go test ./core/resilience/ -run TestGuardTripsAndRecovers`
Expected: FAIL.

- [x] **Step 4: Implement `guard.go` + `shared.go`**

`guard.go`: unexported `guard` struct holding `*gobreaker.CircuitBreaker[struct{}]`,
`*semaphore.Weighted`, config; `Execute` applies Timeout→Retry→Breaker→Bulkhead.
`IsExcluded`: `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)`.
`IsSuccessful`: nil error true; `Options.Breaker.IsSuccessful` hook if set.
`OnStateChange` emits `observability` counter + `log` line when `Provider` set.
`shared.go`: `StateFromBreaker(gobreaker.State) State`, error mapping
(`gobreaker.ErrOpenState` → `ErrOpenState`, `ErrTooManyRequests` →
`ErrTooManyRequests`), bulkhead timeout → `ErrBulkheadFull`, timeout →
`ErrTimeout`.

Run: `GOWORK=off go test ./core/resilience/ -run TestGuardTripsAndRecovers`
Expected: PASS.

- [x] **Step 5: Failing conformance test + edge cases**

`resiliencetest/kit.go`: `RunConformance(t, factory func() resilience.Manager)`
exercising: closed allows, breaker trips, open fails fast, half-open probe,
bulkhead rejects past `MaxConcurrent`, timeout returns `ErrTimeout`, caller
cancellation does not trip. `edge_test.go`: nil factory, duplicate register,
invalid options (negative timeout, ratio >1, MaxConcurrent 0), Guard unknown
name error, Close idempotent.

Run: `GOWORK=off go test ./core/resilience/...`
Expected: FAIL then PASS after implementing.

- [x] **Step 6: Adapter `adapters/resilience/inproc`**

`go.mod` requires `github.com/zenta-dev/zever/core/resilience`,
`github.com/sony/gobreaker/v2 v2.4.0`, `golang.org/x/sync v0.23.0`.
`register.go`: `func Register() { _ = resilience.Register(resilience.Memory,
func(o resilience.Options) (resilience.Manager, error) { return New(o) }) }`.
`inproc.go`: `New(Options) (Manager, error)`; manager holds
`map[string]*guard` + `sync.Mutex`, `Guard(name)` lazily builds from template;
`Close()` closes all. Run kit against it.

Run: `GOWORK=off go -C adapters/resilience/inproc test ./...`
Expected: PASS.

- [x] **Step 7: Commit + PR**

```bash
git add core/resilience adapters/resilience/inproc
git commit -m "feat(resilience): add circuit breaker, bulkhead, and policy composition battery"
```
Run: `tools/pr-runner.sh w1-resilience "feat(resilience): add circuit breaker, bulkhead, and policy composition battery" "feat(resilience): resilience battery" "Adds core/resilience (Guard/Manager, Timeout→Retry→Breaker→Bulkhead) and adapters/resilience/inproc backed by sony/gobreaker/v2 and golang.org/x/sync/semaphore. Conformance kit + edge/bench/example tests."`

---

## Task W1-B: `core/outbox` battery + `adapters/outbox/db` + `memory`

**Worker dir:** `.worktrees/w1-outbox`
**Owns:** `core/outbox/**`, `adapters/outbox/db/**`, `adapters/outbox/memory/**`
**Files:**
- Create: `core/outbox/go.mod`, `doc.go`, `adapter.go`, `errors.go`,
  `options.go`, `message.go`, `outbox.go`, `store.go`, `inbox.go`, `shared.go`
- Create: `core/outbox/outboxtest/kit.go`
- Create tests: `outbox_test.go`, `message_test.go`, `options_test.go`,
  `edge_test.go`, `example_test.go`, `bench_test.go`
- Create: `adapters/outbox/db/{go.mod,doc.go,options.go,register.go,db.go,relay.go,inbox.go,db_test.go,relay_test.go,live_test.go}`
- Create: `adapters/outbox/memory/{go.mod,doc.go,register.go,memory.go,memory_test.go}`

**Interface contract:** exactly as spec Phase B.

- [x] **Step 1: Failing test for Message validation + registry**

```go
// core/outbox/message_test.go
func TestMessageValidate(t *testing.T) {
    if err := (outbox.Message{}).Validate(); !errors.Is(err, outbox.ErrInvalidMessage) {
        t.Fatalf("err = %v, want ErrInvalidMessage", err)
    }
    m := outbox.Message{ID: "e1", Topic: "orders", Payload: []byte("{}")}
    if err := m.Validate(); err != nil { t.Fatalf("valid message err = %v", err) }
}
```
Run: `GOWORK=off go test ./core/outbox/ -run TestMessageValidate` → FAIL.

- [x] **Step 2: Implement adapter/errors/options/message/outbox**

`adapter.go`: `Adapter` (`Memory`, `DB`, `CDC`) + `ParseAdapter`.
`errors.go`: `ErrNilFactory`, `ErrUnknownAdapter`, `ErrDuplicateAdapter`,
`ErrInvalidAdapter`, `ErrInvalidOptions`, `ErrInvalidMessage`,
`ErrTxRequired`, `ErrNotFound`, `ErrStalled`; typed errors.
`options.go`: fields from spec, defaults (`DefaultPollInterval=1s`,
`DefaultBatchSize=100`, `DefaultMaxAttempts=5`, `DefaultRetention=168h`,
`DefaultLockSeconds=30`, `DefaultTable="outbox"`, `DefaultInboxTable="inbox"`),
`Validate()`.
`message.go`: `Message` + `Validate()` (ID, Topic non-empty; ID ≤255 bytes) +
`Clone()`.
`outbox.go`: `Store`/`Publisher`/`Inbox` interfaces, `Status`, `Factory`,
registry `Register`/`Open`/`Default()`.
`store.go`: shared helpers (`validateTopic`, `PublisherFromEventBus`,
`PublisherFromQueue` adapters).
Run: `GOWORK=off go test ./core/outbox/ -run TestMessageValidate` → PASS.

- [x] **Step 3: Failing test for memory adapter atomic Record + relay**

```go
// adapters/outbox/memory/memory_test.go
func TestMemoryRecordAndRelay(t *testing.T) {
    var got []outbox.Message
    pub := outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
        got = append(got, m); return nil
    })
    s, err := memory.New(memory.Options{Publisher: pub})
    if err != nil { t.Fatal(err) }
    defer s.Close()
    if err := s.Record(context.Background(), nil, outbox.Message{ID: "1", Topic: "t", Payload: []byte("x")}); err != nil {
        t.Fatal(err)
    }
    if err := s.Start(context.Background()); err != nil { t.Fatal(err) }
    // memory adapter publishes synchronously on Record; assert immediately
    if len(got) != 1 { t.Fatalf("published %d, want 1", len(got)) }
}
```
Run: `GOWORK=off go -C adapters/outbox/memory test ./...` → FAIL.

- [x] **Step 4: Implement `adapters/outbox/memory`**

`memory.New` stores nothing durable; `Record` validates + publishes via
`Publisher` immediately (dev/test). `Start` no-op, `Status` zero, `Close` nil.
Run: PASS.

- [x] **Step 5: Failing test for db adapter transactional Record + relay**

```go
// adapters/outbox/db/db_test.go
func TestRecordIsTransactional(t *testing.T) {
    conn := mustOpenSQLite(t) // :memory:
    s := mustNewDBStore(t, conn)
    // rollback: Record inside tx then rollback => not published
    tx, err := conn.(db.Transactor).BeginTx(context.Background(), nil)
    if err != nil { t.Fatal(err) }
    if err := s.Record(context.Background(), tx, outbox.Message{ID: "r", Topic: "t", Payload: []byte("x")}); err != nil { t.Fatal(err) }
    _ = tx.Rollback(context.Background())
    var n int
    row := conn.QueryRow(context.Background(), "SELECT COUNT(*) FROM outbox")
    _ = row.Scan(&n)
    if n != 0 { t.Fatalf("rows = %d, want 0 after rollback", n) }
}
```
Run: `GOWORK=off go -C adapters/outbox/db test ./...` → FAIL.

- [x] **Step 6: Implement `adapters/outbox/db`**

`db.go`: `New(Options)` opens via `shared/dbconn.SplitDSN`/`core/db.Open`;
`OpenFromDB(conn, Options)` for `RegisterShared`; migration `CREATE TABLE IF NOT
EXISTS` with columns `(id TEXT PRIMARY KEY, topic TEXT, key TEXT, payload
BLOB/TEXT, headers TEXT, created_at TIMESTAMP, attempts INT, processed_at
TIMESTAMP, locked_until TIMESTAMP, last_error TEXT)` + index
`(processed_at, created_at)`. `Record(ctx, tx db.Tx, msg)` requires non-nil tx
(`ErrTxRequired`) and inserts within it. `relay.go`: `Start` launches goroutine
with `time.Ticker(PollInterval)`; claim with
`SELECT ... WHERE processed_at IS NULL AND (locked_until IS NULL OR locked_until <
now) ORDER BY created_at LIMIT ? FOR UPDATE SKIP LOCKED` (sqlite fallback:
plain UPDATE ... RETURNING claim guarded by a single-writer lock); publish via
`Publisher`, mark processed or increment attempts + backoff (shared/retry);
DLQ after `MaxAttempts` (mark failed, record last_error); retention cleanup
deletes processed older than `Retention`; `traceprop.Inject` into headers before
publish. `Status()` returns counts + `Stalled` when oldest pending older than
`DefaultStallAfter`. `inbox.go`: `Process` inserts `event_id` with `ON CONFLICT
DO NOTHING` in caller tx; if 0 rows, return `nil` (already processed).
`register.go`: `Register()` + `RegisterShared()`.

Run: `GOWORK=off go -C adapters/outbox/db test ./...`
Expected: PASS (sqlite `:memory:`; live postgres test skips without `POSTGRES_DSN`).

- [x] **Step 7: Conformance kit + edge/bench/example, then commit + PR**

`outboxtest/kit.go` runs against any `Store`: Record→publish, retry on publisher
error, DLQ after max attempts, Inbox dedupe. Edge: nil tx → `ErrTxRequired`,
invalid topic, close during relay, double Start.
```bash
git add core/outbox adapters/outbox/db adapters/outbox/memory
git commit -m "feat(outbox): add transactional outbox/inbox battery with db and memory adapters"
```
Run: `tools/pr-runner.sh w1-outbox "feat(outbox): add transactional outbox/inbox battery with db and memory adapters" "feat(outbox): outbox/inbox battery" "Adds core/outbox (Store/Inbox, polling relay) plus adapters/outbox/db (sqlite/postgres, FOR UPDATE SKIP LOCKED, DLQ, traceprop) and adapters/outbox/memory. Conformance kit + tests."`

---

# Wave 1 integration (coordinator)

## Task I1: wire W1 modules into shared files

**Files (coordinator only):**
- Modify: `go.work` (add `./core/resilience`, `./adapters/resilience/inproc`,
  `./core/outbox`, `./adapters/outbox/db`, `./adapters/outbox/memory`)
- Modify: `config/config.go` (add `Resilience Service[resilience.Options]`,
  `Outbox Service[outbox.Options]` fields + `serviceNames` + `RedactedServices`)
- Modify: `config/merge.go`, `config/validate.go`, `config/env.go`
- Modify: `config/README.md` service table
- Modify: `container/container.go` (fields + lazy), `container/services.go`
  (`Resilience()`, `Outbox()` accessors)
- Modify: `container/services_test.go`
- Modify: `docs/src/content/docs/reference/adapters-matrix.mdx`,
  `docs/src/content/docs/reference/api-index.mdx`
- Modify: `CHANGELOG.md` `## [Unreleased]`
- Run: `make deps-sync && make tidy-check && make vet && make build`

- [x] Add go.work entries, run `go build ./...`.
- [x] Add config fields + wiring; `go test ./config/...`.
- [x] Add container accessors; `go test ./container/...`.
- [x] Update docs + CHANGELOG.
- [x] Commit + PR via `tools/pr-runner.sh w1-integration ...`.

---

# Wave 2 (parallel after W1 integration)

## Task W2-A: `shared/grpcclient` + container accessor (coordinator splits)

**Worker dir:** `.worktrees/w2-grpcclient`
**Owns:** `shared/grpcclient/**`
- Create: `shared/grpcclient/{go.mod,doc.go,grpcclient.go,resolver.go,interceptor.go,retry.go,options.go,grpcclient_test.go,resolver_test.go,interceptor_test.go,edge_test.go}`
- Contract: `New(ctx, target, opts...) (*grpc.ClientConn, error)` with
  `round_robin` default service config, static+dns resolvers, otel trace
  interceptor, optional `resilience.Guard` unary interceptor, metadata
  propagation, TLS/insecure options, per-method retry policy JSON merge.
- Tests: static resolver resolves addresses; service config parses;
  interceptor injects trace headers; guard rejects when open; retry policy
  validates `maxAttempts<=5`.
- Commit: `feat(grpcclient): add client-side load balancing, retry, and tracing helper`
- PR via `tools/pr-runner.sh w2-grpcclient ...`
- **Coordinator** adds `container.GRPCClient` accessor in the W2 integration.

## Task W2-B: `adapters/outbox/cdc`

**Worker dir:** `.worktrees/w2-outbox-cdc`
**Owns:** `adapters/outbox/cdc/**`
- Create: `{go.mod,doc.go,options.go,register.go,cdc.go,producer.go,consumer.go,cdc_test.go,live_test.go}`
- Contract: same `outbox.Store`; producer `Emit(ctx, tx, msg)` calls
  `pg_logical_emit_message(true, prefix, payload)` in caller tx; consumer
  `Start` creates slot if missing, `START_REPLICATION ... (proto_version '1',
  publication_names '<pub>', messages 'true')`, decodes
  `pglogrepl.LogicalDecodingMessage` by prefix, publishes, acks LSN; ack after
  publisher success (at-least-once). Requires `wal_level=logical`.
- Live test gated by `POSTGRES_DSN`; unit tests for message framing.
- Commit: `feat(outbox): add postgres logical-replication cdc adapter`
- PR via `tools/pr-runner.sh w2-outbox-cdc ...`

## Task I2: W2 integration

- [x] Add `./adapters/outbox/cdc`, `./shared/grpcclient` to `go.work`.
- [x] Add `container.GRPCClient` accessor + test.
- [x] Add `outbox` cdc adapter to config table/docs; `deps-sync`; build.
- [x] Commit + PR.

---

# Wave 3: Saga on `core/workflow`

## Task W3-A: saga interfaces + adapters

**Worker dir:** `.worktrees/w3-saga`
**Owns:** `core/workflow/**` (saga files only), `adapters/workflow/db/**`,
`adapters/workflow/memory/**`
- Create: `core/workflow/saga.go`, `core/workflow/saga_test.go`,
  `core/workflow/saga_edge_test.go`
- Modify: `adapters/workflow/db/postgres.go` (+`saga.go`, `saga_test.go`,
  `saga_live_test.go`), `adapters/workflow/memory/memory.go`
- Contract: spec Phase D. `RegisterSaga` via type assertion to
  `workflow.SagaRegistrar`; `RunSaga` via `workflow.SagaRunner`.
- DB: tables `workflow_saga_runs(run_id, name, status, current_step, state,
  failed_step, error, updated_at, locked_until)` and
  `workflow_saga_compensations(run_id, step_index, name, attempts, last_error)`.
  Idempotency key `runID:stepIndex` via `core/idempotency`.
- Tests: happy path; failure at step N compensates N-1..0 reverse; pivot stops
  compensation and retries forward; compensation failure recorded + retried;
  stuck run recovered by sweeper.
- Commit: `feat(workflow): add saga orchestration with pivot and compensation`
- PR via `tools/pr-runner.sh w3-saga ...`

## Task I3: W3 integration + docs

- [x] Update `core/workflow/doc.go`, `container/README.md`, docs guides, CHANGELOG.
- [x] `make check`; commit + PR.

---

# Wave 4 — delivered

## Task W4: `.zen` `saga {}` declaration → generated registration

Shipped: `.zen` `saga {}` declaration (DSL + gogen) in #425; resilience redis adapter in #422.

---

## Self-review checklist

- [ ] Every spec section maps to a task (A, B, C, D, integration).
- [ ] No placeholders in interfaces; types consistent across tasks
  (`resilience.Guard`, `outbox.Store`, `workflow.SagaStep`).
- [ ] Each task ends in a commit + PR via `tools/pr-runner.sh`.
- [ ] Coordinator-owned files excluded from worker scopes.
- [ ] `make deps-sync` called after each dependency add.
