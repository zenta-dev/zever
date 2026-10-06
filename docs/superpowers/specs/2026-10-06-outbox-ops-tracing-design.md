# Outbox/Relay Ops + Cross-Service Trace Convention — Design Spec

**Date:** 2026-10-06
**Status:** Implemented — all work shipped (metrics, zever outbox CLI, stalled-relay readiness, inbound extraction + outbound injection, baggage, semconv migration).
**Owner:** @erhahahaa

## Problem

Two production gaps remain after the microservice-readiness layer:

### 1. Cross-service tracing is broken
- `core/middleware.Tracing` (HTTP) and `TracingUnaryServerInterceptor` (gRPC)
  start server spans **without extracting inbound `traceparent`/`tracestate`**,
  so every service starts a fresh root trace.
- `shared/httpclient` never injects `traceprop`, so outbound HTTP calls break the
  trace.
- `shared/traceprop` propagates **TraceContext only** (no Baggage), while the
  OTLP adapter installs a TraceContext+Baggage composite propagator —
  inconsistent.
- `core/observability.Tracer.Start(ctx, name)` takes **no options**, so span
  `SpanKind` (client/server/producer/consumer) and initial attributes cannot be
  expressed; grpcclient/outbox/queue spans default to internal kind.

### 2. Outbox/relay ops is missing
`core/outbox.Status{Pending,Processed,Failed,LastError,Stalled}` and the DB
schema (`status`, `attempts`, `last_error`) exist, but:
- the relay emits **no metrics** (no pending/age/DLQ/publish-latency signals);
- there is **no admin surface** to inspect or replay DLQ rows;
- readiness does not reflect a **stalled** relay;
- there is **no alerting runbook**;
- `zever generate server`/`worker` never start the relay.

## Decisions (approved)

| Decision | Choice |
|---|---|
| Span model | Extend `core/observability` with `StartSpan(ctx,name,opts...)` + `WithSpanKind/WithAttributes/WithLinks`; `Start` stays a wrapper (non-breaking) |
| Inbound extraction | Fix HTTP middleware + gRPC server interceptor to extract before `Start` (behavior change, correct) |
| Outbox admin | `zever outbox` CLI only (no HTTP admin surface) |
| Span attributes | **Migrate now** to OTel semconv names (breaking; CHANGELOG `### Changed` + migration note) |
| Baggage | Switch `traceprop` to TraceContext+Baggage; fixed keys `tenant.id`, `user.id`, `correlation.id` |
| Readiness | Opt-in `outbox.stall_readiness: true` |

## Workstream B — Cross-service trace convention

**B1 — span model (`core/observability`)**
```go
type SpanKind int // Internal, Server, Client, Producer, Consumer
type SpanStartOption func(*spanConfig)
func WithSpanKind(k SpanKind) SpanStartOption
func WithAttributes(attrs ...Attr) SpanStartOption
func WithLinks(links ...SpanLink) SpanStartOption
type Tracer interface {
    Start(ctx context.Context, name string) (context.Context, Span) // unchanged wrapper
    StartSpan(ctx context.Context, name string, opts ...SpanStartOption) (context.Context, Span)
    Shutdown(ctx context.Context) error
}
```
Providers map `SpanKind` to `go.opentelemetry.io/otel/trace.SpanKind`. Noop/stdout
record the kind for assertions.

**B0/B2 — propagation**
- `middleware.Tracing`: `ctx = traceprop.Extract(ctx, headers(r))` before
  `StartSpan(..., WithSpanKind(Server))`.
- `TracingUnaryServerInterceptor`: extract from incoming gRPC metadata.
- `shared/httpclient`: inject `traceprop` from the request context on outbound
  requests (RoundTripper or `Do(ctx, req)` helper).
- `shared/traceprop`: composite `TraceContext + Baggage`; add `WithBaggage(ctx,
  key, val)`, `Baggage(ctx, key)`, and constants `BaggageTenantID = "tenant.id"`,
  `BaggageUserID = "user.id"`, `BaggageCorrelationID = "correlation.id"`. OTLP
  composite propagator already matches.

**B3 — semconv migration (breaking)**
- HTTP server/client: `http.request.method`, `url.path`,
  `http.response.status_code`, `server.address`, `network.protocol.version`.
- gRPC: `rpc.system="grpc"`, `rpc.service`, `rpc.method`, `rpc.grpc.status_code`.
- Messaging: `messaging.system`, `messaging.destination.name`,
  `messaging.operation`, `messaging.message.id`,
  `messaging.message.conversation_id`.
- Resource: `service.version`, `deployment.environment`.
- Counter attribute keys migrate too. Bounded cardinality (no query strings).
- Migration note in the observability guide; `### Changed` in CHANGELOG.

**B4 — docs + conformance**
- `docs/src/content/docs/digging-deeper/cross-service-tracing.mdx`; update
  `docs/production.md` (remove "still evolving"); sidebar; CHANGELOG.
- In-process HTTP→gRPC→HTTP propagation test asserting one trace ID.

## Workstream A — Outbox/relay ops

**A0/A1 — provider injection + metrics**
- `core/outbox/options.go`: `Provider observability.Provider `json:"-"...``,
  `ScopeName = "outbox"`; add `core/observability` dep.
- Emit via `Meter("outbox")`:
  - gauges `outbox.pending`, `outbox.failed`, `outbox.oldest_pending_age_seconds`
  - counters `outbox.published`, `outbox.failed_total`, `outbox.retried`,
    `outbox.relay_errors`
  - histogram `outbox.publish.duration_seconds`
  - attrs `outbox.adapter`, `outbox.table`, `outbox.topic`, `outbox.outcome`
- Spans `outbox.publish` / `outbox.consume` (kinds from B1).
- `container` injects `c.Observability()` into the resolved `Outbox` (optional
  `WithProvider` on adapters).

**A2 — `zever outbox` CLI**
- New separate `outbox.Admin` interface (`List`, `Requeue`, `Purge`) implemented
  by `adapters/outbox/db` (keeps `Store` stable).
- `cmd/zever/outbox.go`: `status [--json]`, `dlq list [--limit]`,
  `dlq requeue (--id|--all)`, `dlq purge --id`, `purge --before <dur>`; DSN/table
  from flags or `zever.yaml`.

**A3 — readiness (opt-in)**
- `outbox.stall_readiness: true` → generated `/readyz` returns 503 and gRPC
  health NOT_SERVING when `Outbox().Status().Stalled`.

**A4 — runbook docs**
- `docs/src/content/docs/digging-deeper/outbox-ops.mdx`: metric list, Prometheus
  alert rules, DLQ runbook, stalled readiness; update `production.md`.

## Cross-cutting

- Conventions: doc comments, no globals/init, ctx first, deterministic tests,
  sentinels `<battery>:`-prefixed, `errors.Is/As`, goimports local prefix.
- Coordinator owns `config/`, `container/`, `cmd/zever` template+goldens, docs
  index, `CHANGELOG`; workers own disjoint module dirs.
- Waves: W1 = B1 ∥ A0/A1; W1 integration = container provider; W2 = B0/B2 ∥ A2;
  W2 integration = cmd dispatch/config; W3 = B3 ∥ A3 ∥ A4/B4.
- Each worker ships a PR via `tools/pr-runner.sh` (watch → fix → squash merge).

## Risks

- B3 breaking attr rename → documented migration; dashboards must update.
- `SpanKind` through the facade is a public API addition → pre-1.0, CHANGELOG.
- Shared-file conflicts → coordinator integration PRs per wave.
