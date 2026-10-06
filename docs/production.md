# Production Deployment

Single operator path. Per-battery docs hold details; this page ties the
deploy-relevant contracts together with file cites. Existing
`docs/src/content/docs/getting-started/deployment.mdx` covers build/config
shapes; this file is the pre/deploy/post gate list.

## Generated server contract

`zever generate server` emits the prod entrypoint
(`cmd/zever/generate_server.go`):

- `GET /healthz` always 200, no dependency checks
  (`generate_server.go:937`).
- `GET /readyz` pings `DB()` with `readyTimeout` and reflects the gRPC
  health serving status; 200 ready, 503 not (`generate_server.go:941-952`).
- Shutdown: `signal.NotifyContext(SIGINT, SIGTERM)` (`:855`), health
  serving status flipped `NOT_SERVING` before
  `grpcServer.GracefulStop()` (`:1012`),
  `httpServer.Shutdown(shutdownCtx)` (`:1017`),
  container close on deferred timeout ctx (`:859-863`).
- HTTP chain when ratelimit enabled:
  `Recover`, `RequestLogger`, `Tracing`, then
  `middleware.RateLimit` (`:928-931`). gRPC chain adds
  `RateLimitUnaryServerInterceptor` (`:905-910`).

Boot, serve, close:

```go
package main

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

func run() error {
	cfg, err := config.Load("/etc/myapp/zever.yaml")
	if err != nil {
		return err
	}
	_ = cfg.RedactedServices()
	c := container.New(cfg)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	}()
	return nil
}
```

`container.Close` closes only resolved services, dependency order,
`errors.Join` on failure (`container/close.go:185`). Never run
`zever dev` (watch) or `zever tinker` (arbitrary-Go eval) against prod.

## Config, Postgres, secrets

- Layering `defaults < file < env`; `config.Default()` uses zero-infra
  adapters plus a deterministic dev crypto key, never prod
  (`config/config.go`, `config/README.md`). Strict files reject typos;
  env override is `<SERVICE>_<FIELD>` (`DB_MAXCONNS`), unparsable values
  fail load.
- Never log raw option maps. Display path is
  `config.Config.RedactedServices` (`config/config.go:139`,
  `config/redact.go:175`).
- Postgres (`adapters/db/postgres/postgres.go`): `pgxpool` pool,
  `MaxConns`/`MinConns`/`MaxConnLifetime`/`MaxConnIdleTime` passthrough
  (`:354-369`); TLS 1.2 floor whenever TLS is enabled (`:350-352`);
  constructor never logs the DSN (`New` doc, `:324-332`). Keep summed
  `MaxConns` across `db`/`search`/`vectorstore` pools below the server
  `max_connections`.
- Secrets: `env` baseline plus real backends (`core/secrets/adapter.go`,
  `adapters/secrets/env/env.go`, `adapters/secrets/vault/`). `env.New`
  requires `Prefix` (`adapters/secrets/env/options.go`); `Get`/`List`
  read prefixed env vars; `Set`/`Delete` always return
  `secrets.ErrNotSupported` (`env.go:55-63`) — rotation means
  redeploy/restart with new env, not an API call. `vault` is the KVv2
  adapter (`Addr`/`Token`/`TokenFile`/`Mount`/`Namespace` in
  `adapters/secrets/vault/options.go`): `Get`/`Set`/`Delete`/`List`
  over HTTP with `DefaultHTTPTimeout`, values never appear in errors.
  Prefer Vault for anything handling payment credentials (SOC2/PCI
  expect a real secrets manager); keep `env` for local dev.
- Crypto: `local` (AES-256-GCM, dev key) plus `kms` envelope adapter
  (`adapters/crypto/kms/`). Envelope format is
  `version||keyID||encrypted-DEK||nonce||ciphertext` (`kms.go:165-185`); `KeyIDs` accepts prior key IDs
  so rotation decrypts old values while new writes use `KeyID`
  (`core/crypto/options.go`). Crypto keys load from secrets, never
  hard-coded (`core/crypto/doc.go`; `security/crypto-secrets.mdx`,
  `digging-deeper/secrets.mdx`).

Secret lookup:

```go
package main

import (
	"context"

	"github.com/zenta-dev/zever/adapters/secrets/env"
	"github.com/zenta-dev/zever/core/secrets"
)

func readSecret(ctx context.Context) ([]byte, error) {
	s, err := env.New(env.Options{Prefix: "APP"})
	if err != nil {
		return nil, err
	}
	_ = secrets.Open
	return s.Get(ctx, "db_password")
}
```

`env.New(env.Options{...})` is the direct constructor; `secrets.Open`
is the registry path once more adapters exist.

## Resilience, outbox, and gRPC clients

- Resilience (`core/resilience`, `digging-deeper/resilience.mdx`): the
  default `memory` adapter keeps every policy process-local, so each
  replica trips its own breaker. Multi-instance deploys set
  `resilience.adapter: redis` so replicas share one circuit per dependency
  (`RedisOptions.Prefix` namespaces the shared state; give each
  environment its own prefix). Guard names and options carry no
  credentials, but never log raw option maps — display path is
  `config.RedactedServices`.
- Outbox (`core/outbox`, `digging-deeper/outbox.mdx`): the default `db`
  adapter is durable (sqlite dev, postgres prod) and its relay claims with
  `FOR UPDATE SKIP LOCKED`, so any number of replicas can relay the same
  table without double-publishing. A container-opened store records but
  cannot `Start` its relay — wire a `Publisher` (eventbus or queue
  bridge) through the adapter constructor. The `cdc` adapter needs
  `wal_level=logical`; live tests are gated by `POSTGRES_DSN`. Consumers
  dedupe through `outbox.Inbox` (or their own idempotency key) — delivery
  is at-least-once. The relay emits `outbox.*` gauges/counters/histogram
  through the observability provider on `outbox.Options.Provider` (nil
  disables telemetry, errors are discarded), so alert rules only work
  with `observability.adapter: otlp`; failures land in the DLQ and are
  repaired with `zever outbox status` → `dlq list` → `dlq requeue
  --id/--all`, while `zever outbox purge --before 168h` trims processed
  history without touching undelivered rows. Full metric names, alert
  rules, and runbook: `digging-deeper/outbox-ops.mdx`.
- gRPC clients (`shared/grpcclient`, `digging-deeper/grpc-client.mdx`):
  `container.GRPCClient(target, opts...)` caches one
  `*grpc.ClientConn` per target string and closes it with the container.
  Credentials are explicit and fail-closed (`ErrNoCredentials`); prod
  passes `WithTLS` with a CA bundle, `WithInsecure` is loopback/dev only.
  The generated server registers the standard gRPC Health Checking
  Protocol: `/readyz` reflects the serving status and the server flips
  `NOT_SERVING` before `GracefulStop`, so load balancers stop routing
  before in-flight RPCs are dropped.

## Rate limiting

Actual wiring is `core/middleware` over `core/ratelimit`, not router
middleware: `middleware.RateLimit` (HTTP, 429 + `Retry-After`,
fail-open default, `WithFailMode(FailClosed)` opt-in) and
`RateLimitUnaryServerInterceptor` (gRPC, `ResourceExhausted`)
(`core/middleware/ratelimit.go:88-158`). Backends:
`adapters/ratelimit/memory`, `adapters/ratelimit/redis`
(`core/ratelimit/options.go`: `Rate`, `Burst`, `IdleTTL`).
Generated servers wire it only when the schema enables it
(`generate_server.go:884-889,929-931`). See
`digging-deeper/ratelimit.mdx`, `basics/middleware.mdx`.

## Migrations

`orm/migrate` computes and executes live-schema diffs; DDL renders via
`dsl/backend/atlas` (`orm/migrate/apply.go:11`): `Plan` (compute, no
side effects, `orm/migrate/diff.go:206`), `Apply` (checksum-recorded
execute, `apply.go:194`), `ComputeRollback` / `ApplyRollback`
(`rollback.go:112,382`). Dialects `postgres`/`sqlite`/`mysql` only;
anything else is `ErrUnsupportedDialect`. `zever db migrate` /
`zever db rollback` are thin wrappers over this package
(`orm/migrate/doc.go`). See `database/migrations-seeding.mdx`.

Apply a computed plan:

```go
package main

import (
	"context"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/orm/migrate"
)

func applyPending(ctx context.Context, c *container.Container, plan *migrate.MigrationPlan) (int, error) {
	db, err := c.DB()
	if err != nil {
		return 0, err
	}
	return migrate.Apply(ctx, db, plan)
}
```

## Workflows

`memory` is in-process only: every in-flight run is lost on restart or
deploy (`adapters/workflow/memory/doc.go`). Anything business-critical
runs on `postgres` (`adapters/workflow/db/`): DB-backed runs table
with lease-based crash recovery and idempotency keys, so a second
replica reclaims expired leases after a crash. `core/workflow.Options`
carries `DSN`/`Table` (`core/workflow/options.go`, following the
`core/queue` `RedisOptions` precedent); empty DSN selects sqlite for
dev, set DSN opens postgres for durable runs. Resolve via
`container.Workflow()` like every other battery — `memory` is the
`config.Default()` adapter, never prod.

## Observability

OTLP gRPC adapter (`adapters/observability/otlp/otlp.go:451 `New`):
sets W3C `TraceContext` + `Baggage` composite propagator globally
(`otlp.go:43-57`); `middleware.Tracing` starts one span per request,
path-only span names (no query strings), propagates ctx to handlers
(`core/middleware/tracing.go`); `MapCarrier`
(`core/observability/context.go`) bridges non-HTTP carriers.
`spancheck` lint is on (`.golangci.yml:47`); the otlp `Start` handoff
carries a `//nolint:spancheck` with owner-End justification
(`otlp.go:142`).

Cross-service propagation follows one contract: extract inbound
W3C `traceparent`/`tracestate`/`baggage` **before** the server span
starts (`middleware.Tracing`, `TracingUnaryServerInterceptor`), inject
the current context on every outbound call (`shared/httpclient`
transport, `shared/grpcclient` interceptors), and continue stored
context on the async path (`core/outbox` `Record` headers →
`traceprop.StartConsumeSpan`). Because extraction precedes the span, a
server span joins the caller's trace instead of starting a fresh root.
Baggage carries three fixed keys — `tenant.id`, `user.id`,
`correlation.id` — set once at ingress and read downstream; values are
untrusted, short, and never secrets. Span kinds (`internal`/`server`/
`client`/`producer`/`consumer`) ride `observability.StartSpan` via
`WithSpanKind`, with `Tracer.Start` as the internal-span wrapper.
Attribute keys are migrating to OTel semconv names — `http.method` →
`http.request.method`, `http.path` → `url.path`, `http.status_code` →
`http.response.status_code`, gRPC `rpc.*`, messaging `messaging.*` —
so saved dashboards and alert rules on the old keys need updating.
Verify in the collector, not by inspection: one request through two
services must resolve to a single trace ID. Prod sets
`observability.adapter: otlp` with `endpoint: host:port` +
`service_name`; `stdout` is dev-only. See
`digging-deeper/cross-service-tracing.mdx` and
`digging-deeper/observability.mdx`.

## Gates and supply chain

`.github/workflows/ci.yml`: `test` (fast gate, vet+tidy+test+build),
`test-race` (race everywhere; PRs run `test-race-fast` no-coverage,
non-PR `main`/`tags` runs `test-race-all` with atomic coverage +
Codecov upload, `ci.yml:243-291`), `lint` (golangci-lint), `vulnerability`
(`govulncheck`, informational on PRs, blocking on main), `sbom`
(CycloneDX per affected module), `doc-snippets` (this file's go blocks
plus `GOWORK=off go -C docs/examples test`). Pins
(`Makefile:9-11`): golangci-lint `v2.13.2`, govulncheck `v1.8.0`,
cyclonedx-gomod `v1.12.0`.

Worked image: `examples/showcase/Dockerfile` (multi-stage:
`golang:1.27.1-bookworm` builder emitting a `CGO_ENABLED=0` static binary
via `-trimpath -ldflags="-s -w"`, final
`gcr.io/distroless/static-debian13:nonroot` with absolute `ENTRYPOINT`).
Build from the repo root (`docker build -f examples/showcase/Dockerfile
.`); the builder runs with `GOWORK=off`, so only the showcase module plus
its replace targets ship in the build context. The worker follows the same
pattern (`./cmd/worker`, commented in the Dockerfile). Static works because
sqlite is pure-Go (no cgo) and the static distroless base carries CA certs
for stripe/cloud TLS. One container per process; other build shapes
(`make build`, `GOOS=linux go build`, systemd unit) stay in
`deployment.mdx` variants. Newcomers clone `examples/showcase` first:
it is the reference app behind this page.

## Release verification

Tagged releases (`v*`) build static binaries and SBOMs via
`.github/workflows/release.yml` (builder flags mirror the Dockerfile
above: `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`), attest both
with SLSA provenance, and attach them to the GitHub release. Verify
before deploying:

```sh
gh attestation verify dist/zever-linux-amd64 --repo zenta-dev/zever
gh attestation verify sbom/cmd_zever.json --repo zenta-dev/zever
```

## Checklists

### Pre-deploy

- [ ] Config file strict-loads (`config.Load` explicit path); no
  `config.Default()` crypto/JWT keys; `log` is `slog`/`zerolog`;
  `observability` is `otlp` with endpoint + service name.
- [ ] `DB_DSN` + `DB_MAXCONNS` set; summed pool sizes below server
  `max_connections`; TLS on (or explicit `sslmode=disable` only for
  ephemeral envs).
- [ ] Secrets only via env/mounted files; boot path prints
  `RedactedServices`, never raw maps.
- [ ] `migrate.Plan` reviewed against live DB; rollback plan computed
  (`ComputeRollback`) and kept with the release.
- [ ] Rate-limit `Rate`/`Burst` set for public routes; fail mode chosen
  per endpoint sensitivity.
- [ ] Resilience on `redis` when multi-instance (shared breaker state);
  per-dependency guard names chosen; `IsSuccessful` classifies
  business-error successes.
- [ ] Outbox on `db` (postgres in prod) with a `Publisher` wired through
  the adapter constructor; consumer-side inbox/idempotency keys chosen.
- [ ] Outbox alerting armed on the `outbox.*` series (pending growth,
  oldest-pending age over the 5m `db.DefaultStallAfter`, `failed_total`
  increase, `relay_errors` rate, publish p99); runbook link reachable
  from the alert — `digging-deeper/outbox-ops.mdx`.
- [ ] `outbox.stall_readiness` decided deliberately: `true` pulls a
  stalled replica out of rotation via `/readyz` 503 + gRPC
  `NOT_SERVING`; off when replicas share a table and shedding traffic is
  worse than alerting.
- [ ] Trace propagation verified end to end: one request spanning two
  services resolves to a single trace ID, outbound client spans are
  children of their server span, and the three baggage keys
  (`tenant.id`, `user.id`, `correlation.id`) are set at ingress. Saved
  queries/dashboards updated for the semconv attribute rename
  (`http.method` → `http.request.method`, `http.path` → `url.path`,
  `http.status_code` → `http.response.status_code`).
- [ ] Binary built from clean tree at a tag; SBOM artifact kept;
  `govulncheck` clean.

### Deploy

- [ ] Rolling update, one process per container; new pods pass `/readyz`
  before receiving traffic.
- [ ] Apply migrations (`migrate.Apply` / `zever db migrate`) before or
  with the rollout per the release plan, never after traffic shifts to
  code that needs the new schema.

### Post-deploy verification

```sh
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
zever db migrate --dry-run   # expect: no pending statements
```

- [ ] `/healthz` 200, `/readyz` 200 (503 = DB ping failing or gRPC health
  not serving, hold rollout); health serving status flips `NOT_SERVING`
  on shutdown before `GracefulStop`.
- [ ] Migration bookkeeping shows the release checksum recorded; re-run
  is a no-op.
- [ ] Traces arriving in collector; 5xx span errors triaged.
- [ ] Trace continuity confirmed after rollout: one sampled request
  across services shows a single trace ID with correct parent/child
  links, and no service starts a fresh root. A two-root trace means the
  inbound `traceparent` was not extracted — see
  `digging-deeper/cross-service-tracing.mdx`.
- [ ] Relay signals healthy: `outbox.pending` flat, `outbox.failed` at
  0, `outbox.relay_errors` silent; `zever outbox status` shows
  `stalled false`. A non-empty DLQ is triaged through
  `digging-deeper/outbox-ops.mdx` (fix root cause, then
  `zever outbox dlq requeue --all`), never purged blind.
- [ ] Secret rotation drill: stage new `APP_db_password` (or
  `DB_DSN`/`AUTH_JWT_SECRET`) in the supervisor, restart one replica,
  confirm `/readyz` 200 with old value revoked; `Set`/`Delete` are
  unsupported on the env adapter by design.
- [ ] `SIGTERM` drill: `kill -TERM`, confirm `Shutdown` +
  container `Close` complete within grace period, buffered spans flush.

## Benchmarks

Measured router/ORM/queue baselines plus repro commands live in
`docs/benchmarks.md`. Numbers are a point-in-time snapshot, not a deploy
gate; re-run locally before drawing conclusions.

## See also

- `docs/src/content/docs/getting-started/deployment.mdx` (build shapes)
- `config/README.md`, `container/README.md`, `cmd/zever/README.md`
- `database/migrations-seeding.mdx`, `digging-deeper/observability.mdx`,
  `digging-deeper/cross-service-tracing.mdx`,
  `digging-deeper/outbox-ops.mdx`,
  `digging-deeper/ratelimit.mdx`, `digging-deeper/secrets.mdx`,
  `security/crypto-secrets.mdx`
