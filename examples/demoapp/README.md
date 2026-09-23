# demoapp — batteries-included Zever showcase

A fully functional demo app that exercises **every battery** in `zever` via
HTTP and background jobs. One `.zen` schema drives the ORM, Protobuf API and
SQL schema; `container.Container` wires all adapters.

This app runs with **zero external services** — every battery uses its
in-memory/local adapter by default (sqlite files under `data/`). Swap
adapters in `zever.yaml` or via env (`AUTH_JWT_SECRET`, `DB_MAXCONNS`, …)
for production.

> Split with `examples/showcase`: **demoapp = battery breadth over HTTP**
> (one endpoint per battery, thin handlers), **showcase = DSL depth** (every
> schema feature — enums, relations, messages, paginated RPCs — in one
> domain, with HTTP + gRPC transports sharing one service impl).

## What it demonstrates

| Battery | Adapter (demo) | Showcase endpoint |
|---------|---------------|-------------------|
| **db** (sqlite) | `sqlite` | CRUD via zenorm (`/v1/products`, `/v1/orders`, `/v1/posts`) — `From`, `Where`, `InsertInto`, `OrderBy`, `Limit` |
| **auth** (jwt) | `jwt` | `POST /register`, `POST /login` → JWT, `GET /me` |
| **cache** (memory) | `memory` | `POST/GET /demo/cache/{key}` |
| **flag** (static) | `static` (`flags.json`) | `GET /demo/flag/{key}` |
| **permission** (rbac) | `rbac` | `POST /demo/permission/check` |
| **ratelimit** (memory) | `memory` | `POST /demo/ratelimit/{key}` |
| **lock** (memory) | `memory` | `POST /demo/lock/{key}` |
| **idempotency** (memory) | `memory` | `POST /demo/idempotency/{key}` |
| **session** (memory) | `memory` | `POST /demo/session`, `GET /demo/session/{id}` |
| **queue** (memory) | `memory` | `POST /demo/queue/{topic}` + `POST /demo/jobs/dispatch/{name}` |
| **job + scheduler** (embedded) | `embedded` | 4 jobs (`SendWelcomeEmail`, `ProcessOrder`, `GenerateDailyReport`, `ReindexSearch`) + 2 schedules (`DailyReport` 09:00, `HourlyReindex` hourly) — see `cmd/worker` |
| **eventbus** (memory) | `memory` | `POST /demo/eventbus/publish` |
| **search** (sqlite) | `sqlite` (`data/search.db`) | `POST /demo/search/index`, `GET /demo/search?q=` |
| **vectorstore** (sqlite) | `sqlite` (`data/vectors.db`, dims 8) | `POST /demo/vector/upsert`, `POST /demo/vector/query` |
| **storage** (local) | `local` (`tmp/demo-storage`) | `POST /demo/storage/presign` |
| **media** (local) | `local` (`tmp/demo-media`) | `POST /demo/media/upload` |
| **ai** (anthropic) | `anthropic` | `POST /demo/ai/generate` (needs credentials; 502 without, server stays up) |
| **geo** (static) | `static` (`data/cities.json`) | `POST /demo/geo/geocode` |
| **i18n** (embed) | `embed` (`locales/*.json`) | `GET /demo/i18n/{locale}/{key}` |
| **crypto** (local) | `local` | `POST /demo/crypto/encrypt\|decrypt` |
| **secrets** (env) | `env` | `GET /demo/secrets/{name}` |
| **notification** (log) | `log` | `POST /demo/notification` |
| **webhook** (http) | `http` | `POST /demo/webhook/register` |
| **workflow** (memory) | `memory` | `POST /demo/workflow/start` (`demo-workflow` echo step registered at boot) |
| **analytics** (log) | `log` | traced in `POST /register` |
| **billing/payment** (stub) | `stub` | wired, no HTTP demo (stub returns mock) |
| **document** (local) | `local` | `POST /demo/document/render` (needs a local browser; 502 without, server stays up) |
| **observability** (stdout) | `stdout` | `GET /demo/observability` |
| **tenant** (single) | `single` | `GET /demo/tenant` |
| **log** (slog) | `slog` | all handlers via `Server.Log` |
| **router** (stdhttp) | `stdhttp` | all routes via `router.Router` |
| **mailer** (log) | `log` | wired; the worker's welcome-email job sends through it |

Not showcased: **realtime** — zever has no realtime battery (no such service
in the container), so there is nothing to wire. The reference zen-go
demoapp's `/demo/realtime/publish` has no equivalent here.

Schema-first DSL: [`schema/app.zen`](schema/app.zen) defines 7 entities
(`User`, `Category`, `Product`, `Order`, `OrderItem`, `Post`, `Comment`),
3 services, 4 jobs, 2 schedules.

## Quickstart

Run from `examples/demoapp/` so `zever.yaml` and `data/` resolve:

```bash
# 1. Generate all backends (already committed under generated/; re-run to verify, from repo root)
zever compile examples/demoapp/schema/app.zen --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out examples/demoapp/generated

# 2. Create sqlite DBs (from examples/demoapp/)
zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen

# 3. Seed demo data (idempotent; needs no JWT secret)
go run ./db/seed

# 4. Run API (in one shell)
go run ./cmd/server          # :8080 (-addr to change)

# 5. Run worker + scheduler (in another shell)
go run ./cmd/worker
```

Exercise:

```bash
curl -s localhost:8080/health | jq
curl -s -X POST localhost:8080/register -H 'content-type: application/json' -d '{"email":"ada@demo.test","password":"demo-pass-12","name":"Ada"}' | jq
TOKEN=$(curl -s -X POST localhost:8080/login -H 'content-type: application/json' -d '{"email":"admin@demo.test","password":"demo-pass-12"}' | jq -r .token)

curl -s localhost:8080/v1/products | jq
curl -s localhost:8080/demo/cache/hello -X POST --data-binary "world" | jq
curl -s localhost:8080/demo/cache/hello | jq
curl -s localhost:8080/demo/flag/new_checkout | jq
curl -s localhost:8080/demo/i18n/en/hello | jq
curl -s localhost:8080/demo/i18n/fr/hello | jq
curl -s -X POST localhost:8080/demo/crypto/encrypt -H 'content-type: application/json' -d '{"plaintext":"hello"}' | jq

# worker logs on schedule:
# INFO demo worker started jobs=4 schedules=2
# INFO job ReindexSearch msg="search index refreshed"
```

Production secrets (dev defaults are insecure by design and marked as
such in `internal/app/app.go`):

```bash
export AUTH_JWT_SECRET="$(head -c 32 /dev/urandom | base64)" # >=32 bytes
# swap cache to redis, queue to redis, etc. via zever.yaml
```

## Regenerating code

`generated/` is CLI output — never hand-edit. From the repo root:

```bash
zever compile examples/demoapp/schema/app.zen --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out examples/demoapp/generated
```

`internal/api/testdata/schema.sql` is the DDL from
`zever db migrate --dry-run --adapter=sqlite` (run from `examples/demoapp/`).

Hand-owned, never overwritten: `internal/app/app.go` (adapter selection,
dev JWT secret, RBAC rules, i18n/storage/search/vector/geo/flag runtime
defaults), `internal/service/jobs/*` bodies, `internal/service/seed/seed.go`
bodies, `internal/api/*` handlers, schema, `zever.yaml`, locales/fixtures,
tests. `db/seed/main.go` loads config directly so seeding needs no JWT
secret.

## Tests

```bash
go test -count=1 ./examples/demoapp/...
```

Black-box HTTP tests over httptest with a fresh sqlite file per test: full
register→login→product→order lifecycle, per-account isolation,
tampered-token rejection, posts, and one subtest per demo battery.
Deterministic only: no sleeps, no network, no unseeded randomness, no
timing assertions. The AI and document endpoints are asserted
route-present (their success paths need external credentials/a browser).

## Files

- `schema/app.zen` — source of truth
- `generated/` — CLI output (`atlas,gogen,openapi,proto,protogogen,zenorm`)
- `internal/app/app.go` — config layering + demo defaults
- `internal/api/*.go` — handlers for every battery
- `internal/service/jobs/jobs.go` — one handler per declared job
- `internal/service/seed/seed.go` — idempotent demo data via zenorm
- `locales/en.json`, `locales/fr.json` — embedded i18n catalogs
- `flags.json`, `zever.yaml` — flag & config demo
- `data/cities.json` — static geo fixture (`*.db` files are gitignored)
- `cmd/server/main.go`, `cmd/worker/main.go`, `db/seed/main.go` — thin entrypoints
