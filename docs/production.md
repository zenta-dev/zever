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
- `GET /readyz` pings `DB()` with `readyTimeout`; 200 ready, 503 not
  (`generate_server.go:941-952`).
- Shutdown: `signal.NotifyContext(SIGINT, SIGTERM)` (`:855`),
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
- Secrets: only the `env` adapter ships (`core/secrets/adapter.go`,
  `adapters/secrets/env/env.go`). `New` requires `Prefix`
  (`adapters/secrets/env/options.go`); `Get`/`List` read prefixed env
  vars; `Set`/`Delete` always return `secrets.ErrNotSupported`
  (`env.go:55-63`) — rotation means redeploy/restart with new env, not
  an API call. `core/secrets/options.go` reserves `Addr`/`Token`/`Mount`
  (Vault) and `ProjectID`/`Region` (GCP/AWS) for future backends; no
  Vault/KMS adapter exists yet. Crypto keys load from secrets, never
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

## Observability

OTLP gRPC adapter (`adapters/observability/otlp/otlp.go:451 `New`):
sets W3C `TraceContext` + `Baggage` composite propagator globally
(`otlp.go:43-57`); `middleware.Tracing` starts one span per request,
path-only span names (no query strings), propagates ctx to handlers
(`core/middleware/tracing.go`); `MapCarrier`
(`core/observability/context.go`) bridges non-HTTP carriers.
`spancheck` lint is on (`.golangci.yml:47`); the otlp `Start` handoff
carries a `//nolint:spancheck` with owner-End justification
(`otlp.go:142`). Cross-service propagation convention beyond otel
defaults is still evolving — rely on otel globals, verify in the
collector. Prod sets `observability.adapter: otlp` with
`endpoint: host:port` + `service_name`; `stdout` is dev-only. See
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

No `Dockerfile`/distroless base is checked in. Ship a static binary
per `deployment.mdx` variants (`make build`, `GOOS=linux go build`,
systemd unit, K8s image with `ENTRYPOINT`). One container per process.

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

- [ ] `/healthz` 200, `/readyz` 200 (503 = DB ping failing, hold rollout).
- [ ] Migration bookkeeping shows the release checksum recorded; re-run
  is a no-op.
- [ ] Traces arriving in collector; 5xx span errors triaged.
- [ ] Secret rotation drill: stage new `APP_db_password` (or
  `DB_DSN`/`AUTH_JWT_SECRET`) in the supervisor, restart one replica,
  confirm `/readyz` 200 with old value revoked; `Set`/`Delete` are
  unsupported on the env adapter by design.
- [ ] `SIGTERM` drill: `kill -TERM`, confirm `Shutdown` +
  container `Close` complete within grace period, buffered spans flush.

## See also

- `docs/src/content/docs/getting-started/deployment.mdx` (build shapes)
- `config/README.md`, `container/README.md`, `cmd/zever/README.md`
- `database/migrations-seeding.mdx`, `digging-deeper/observability.mdx`,
  `digging-deeper/ratelimit.mdx`, `digging-deeper/secrets.mdx`,
  `security/crypto-secrets.mdx`
