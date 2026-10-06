# Outbox/Relay Ops + Cross-Service Trace Convention — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `subagent-driven-development`. Steps use `- [ ]`.

**Goal:** fix cross-service trace propagation and add outbox/relay observability + ops tooling.

**Spec:** `docs/superpowers/specs/2026-10-06-outbox-ops-tracing-design.md`

**Tech:** Go 1.27, `go.opentelemetry.io/otel` (already in graph), `core/observability`, `shared/traceprop`, `shared/httpclient`, `core/outbox`, `adapters/outbox/{db,cdc}`, `cmd/zever`.

**Worker protocol:** one subagent per disjoint module dir; run `tools/pr-runner.sh <branch> "<commit>" "<title>" "<body>"` which pushes, opens the PR, watches CI, fixes on red (≤5), squash-auto-merges on green. Coordinator owns shared files (`config/`, `container/`, `cmd/zever` template/goldens, docs index, `CHANGELOG`) and runs one integration PR per wave.

---

## Wave 1 (parallel)

### Task B1 — extend `core/observability` span model
**Files:** `core/observability/observability.go`, `adapters/observability/{otlp,stdout,noop}/*.go`, `core/observability/observabilitytest/`, tests/bench.

- [ ] Failing test: `StartSpan(ctx, "s", WithSpanKind(Server), WithAttributes(String("k","v")))` reaches the provider with kind+attrs.
- [ ] Add `SpanKind` (Internal/Server/Client/Producer/Consumer), `SpanStartOption`, `WithSpanKind/WithAttributes/WithLinks`, `Tracer.StartSpan(ctx,name,opts...)`; make `Start` call `StartSpan`.
- [ ] OTLP maps kind→`trace.WithSpanKind`, attrs→`trace.WithAttributes`; stdout/noop record kind/attrs for assertions.
- [ ] Conformance kit asserts kind/attrs on all three adapters.
- [ ] `GOWORK=off go test ./...` + `-race` in `core/observability` and each adapter; `gofmt -l` empty.
- [ ] Commit + `pr-runner.sh w1-observability ...`.

### Task A0/A1 — outbox provider injection + relay metrics
**Files:** `core/outbox/options.go`, `core/outbox/outbox.go` (ScopeName), `core/outbox/go.mod`, `adapters/outbox/db/{relay.go,options.go}`, `adapters/outbox/cdc/consumer.go`, tests.

- [ ] Add `Provider observability.Provider `json:"-"...`` to `core/outbox.Options`; `ScopeName="outbox"`; require+replace `core/observability`; `GOWORK=off go mod tidy`.
- [ ] Failing test with a recording `observability.Metrics`: successful publish increments `outbox.published` + records `outbox.publish.duration_seconds`; failure increments `outbox.retried` then `outbox.failed_total`; `Status()`/poll emits `outbox.pending` and `outbox.oldest_pending_age_seconds` gauges.
- [ ] Emit metrics in db `deliver`/`markProcessed`/`markFailed`/`markRetry`/`Status` and cdc consumer; attrs `outbox.adapter/table/topic/outcome`; no-op when Provider nil.
- [ ] Spans `outbox.publish`/`outbox.consume` (use B1 `StartSpan` if present; otherwise `Start`).
- [ ] Tests: db sqlite `:memory:` + recording meter; cdc consumer seam. `go test ./...` in both adapters.
- [ ] Commit + `pr-runner.sh w1-outbox-metrics ...`.

### Integration I1 (coordinator)
- [ ] `container` injects `c.Observability()` into resolved `Outbox` via optional `WithProvider`; config README; `make check-all` + `modgraph-check` + `deps-sync-check`; PR.

---

## Wave 2 (parallel)

### Task B0/B2 — propagation core
**Files:** `core/middleware/tracing.go` (+tests), `shared/traceprop/traceprop.go` (+tests), `shared/httpclient/httpclient.go` (+tests), `shared/httpclient/go.mod`.

- [ ] `middleware.Tracing`: `ctx = traceprop.Extract(ctx, headers)` before `StartSpan(..., Server)`; semconv attrs (B3 may follow).
- [ ] gRPC server interceptor: extract incoming metadata; `Server` kind.
- [ ] `traceprop`: composite `TraceContext+Baggage`; add `WithBaggage/Baggage` + `BaggageTenantID/UserID/CorrelationID`.
- [ ] `httpclient`: inject traceprop from ctx on outbound (`Do(ctx,req)` + RoundTripper); require+replace `shared/traceprop`.
- [ ] e2e test: in-process HTTP server→client and gRPC server→client, assert same trace ID; baggage round-trips.
- [ ] Commit + `pr-runner.sh w2-propagation ...`.

### Task A2 — `zever outbox` CLI
**Files:** `cmd/zever/outbox.go` (+test), `cmd/zever/*` dispatch registration, `core/outbox/admin.go`, `adapters/outbox/db/admin.go`.

- [ ] `outbox.Admin` interface (`List`, `Requeue`, `Purge`) + db implementation (`status='failed'` DLQ; requeue resets `status/attempts/locked_until`; purge deletes).
- [ ] CLI subcommands + `--json`; DSN/table from flags or `zever.yaml`; errors to stderr.
- [ ] sqlite `:memory:` tests: seed rows, assert status counts, requeue, purge.
- [ ] Commit + `pr-runner.sh w2-outbox-cli ...`.

### Integration I2 (coordinator)
- [ ] Register `outbox` command in `cmd/zever` dispatch/help; config docs; PR.

---

## Wave 3 (parallel)

### Task B3 — semconv migration (breaking)
**Files:** `core/middleware/tracing.go`, `shared/grpcclient/interceptor.go`, `adapters/outbox/{db,cdc}`, `adapters/queue/*`, `adapters/eventbus/*`, `core/observability/attr.go`.

- [ ] Add semconv key consts; replace `http.method`/`http.path`/`http.status_code` with `http.request.method`/`url.path`/`http.response.status_code`; gRPC `rpc.*`; messaging `messaging.*`.
- [ ] Update counter attr keys; update tests; `CHANGELOG` `### Changed` + migration note in observability guide.
- [ ] `make check-all` touched modules; `pr-runner.sh w3-semconv ...`.

### Task A3 — readiness on stall (opt-in)
**Files:** `config/*` (option), `cmd/zever/generate_server.go` template + goldens + tests.

- [ ] `outbox.stall_readiness` option; generated `/readyz` 503 + gRPC NOT_SERVING when `Status().Stalled`.
- [ ] Golden regen via test `-update-golden`; `make check-all MODULES="cmd/zever config"`.
- [ ] Commit + `pr-runner.sh w3-readiness ...`.

### Task A4/B4 — docs + conformance
**Files:** `docs/src/content/docs/digging-deeper/{outbox-ops,cross-service-tracing}.mdx`, `docs/astro.config.mjs`, `docs/production.md`, `CHANGELOG.md`.

- [ ] outbox-ops: metrics, Prometheus alert rules, DLQ runbook, stalled readiness.
- [ ] cross-service-tracing: propagation rules, span kinds/names/attrs, baggage keys, provider setup.
- [ ] `tools/check-doc-snippets.sh` clean; `pr-runner.sh w3-docs ...`.

### Integration I3 (coordinator)
- [ ] Docs sidebar/index, `production.md` final, `CHANGELOG`, full `make check-all` on all touched modules; PR.

---

## Self-review
- [ ] Every spec section maps to a task.
- [ ] Worker scopes disjoint; coordinator owns shared files.
- [ ] `make deps-sync` after adding `core/observability`/`shared/traceprop` deps.
- [ ] Breaking attr rename documented.
