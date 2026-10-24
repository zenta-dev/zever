# Showcase example

Shop example built on the zever framework. Exercises every DSL
capability in one schema: all eleven scalars, inline + named enums,
optional fields, defaults, all four relation kinds, indexes, `@schema`,
`@renamed_from`, messages (request + nested + returns), all HTTP verbs,
every auth shape, permission checks, error sets, paginated RPCs, jobs with
params, and schedules.

## Services (zero-infra)

Every wired adapter is zero-infra — sqlite plus in-memory and
local backends — so showcase runs with no external services and ships no
`compose.yaml`. `zever new` only generates a compose file when a picked
battery needs Postgres (`db`, `search`, `vectorstore`, `workflow`) or
Redis (`cache`, `queue`, `session`, `ratelimit`, `lock`, `eventbus`,
`idempotency`); none of those are selected here.

## Wiring (CLI owns infra, developer owns logic)

`cmd/server` mirrors `zever generate server`: HTTP (`:8080`) + gRPC
(`:9090`) from one container, schema routes and services registered via
generated `RegisterModule`, auth enforced identically on both transports
(`GRPCPolicies` + `authz`). `cmd/worker` mirrors
`zever generate worker`: one `job.Register` per declared job, one
`sched.Schedule` per schedule. Byte-delta vs generator output is limited
to: module path, the marked `/api` mount block + `q`/`i18n`/`flags`/`hasher`
resolves, `net.ListenConfig` (repo noctx lint), `svcImpl` Deps construction
in server; Deps prelude + Logger + queue order in worker.

Business logic lives in exactly one place:
`internal/service/shop/shop_service.go` (`ShopServiceImpl`, 9 RPCs).
The `/api/*` routes are thin adapters over the same impl (richer shapes:
register/login, item lists), and `ContextWithSubject` threads the JWT
subject into impl context (generated wrappers supply `authz` claims
instead). Write logic once; both transports share it.

## Regenerating

CLI-owned, regenerate anytime: `generated/` (via `zever compile` from repo
root), `internal/api/testdata/schema.sql` (via `zever db migrate --dry-run`),
entrypoint structure (`cmd/server`, `cmd/worker` — regenerate in a scratch
project with `go.mod` since examples carry none, then copy).
Hand-owned, never overwritten: `internal/app/app.go` (adapter wiring, JWT
gate, i18n/permission runtime defaults), `internal/service/shop/shop_service.go`
(9 RPC bodies), `internal/service/jobs/*` bodies (Deps-closure = filled-in stub
form), `internal/service/seed/seed.go` bodies, `internal/api/*` rich shapes,
schema, service env, locales/fixtures, tests.
`db/seed/main.go` intentionally bypasses `app.New()` (`config.Resolve` directly)
so seeding needs no JWT secret.

## Layout

- `schema/shop/shop.zen` — the whole domain (User, Profile, Category,
  Product, Tag, Order, OrderItem, Review; ShopService; 4 jobs; 2 schedules).
- `generated/` — CLI output (`--backend=atlas,gogen,openapi,proto,protogogen,zenorm`).
  Never hand-edit; regenerate from the repo root with:
  `zever compile examples/showcase/schema/shop/shop.zen --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out examples/showcase/generated`
  (run from the root so gogen/protogogen import paths resolve).
- `internal/api/testdata/schema.sql` — DDL extracted from
  `zever db migrate --dry-run --adapter=sqlite`.
- `internal/api/` — `/api/*` adapters over the same impl (register/login,
  item-list checkout, reviews) with auth, rbac, ratelimit, queue, i18n, flag.
- `internal/service/shop/` — `ShopServiceImpl`: the single business-logic
  home all transports share (+ cross-transport parity tests).
- `internal/service/seed/` — idempotent demo data via the zenorm backend.
- `internal/service/jobs/` — one `Handle*` handler per declared job
  (`generate worker` naming).
- `.env.example` — every service env key with its default.
- `data/` — local sqlite path (git-kept empty) + static geo fixture.

## DSL coverage map

| Feature | Where |
|---|---|
| scalars (11) | User fields |
| named `enum` | OrderStatus, Role (+ defaults) |
| inline `enum()` | Order.priority |
| `?` optional + `@default` | User.nickname, Order.note |
| `has_many` / `belongs_to` | User/Order/OrderItem/Review graph |
| `has_one` (`@unique` FK) | User.profile <-> Profile |
| `many_to_many` + reciprocal `join_table` | Product.tags <-> Tag.products |
| `index()` + `@unique` index | Product, Order |
| `@schema` | Order → billing |
| `@renamed_from` | Product.headline (was title) |
| `message` request + nested + returns | CreateOrderRequest, CheckoutRequest, OrderReceipt |
| verbs GET/POST/PUT/PATCH/DELETE | ShopService RPCs |
| `auth: none/required/roles` | GetProduct / ListProducts / CreateProduct |
| `permission: check` | DeleteProduct |
| `errors:` sets | Create/Update/Patch/Delete/Checkout |
| `paginated: true` | ListProducts, ListOrders |
| job params + queues + retry shapes | SendConfirmation / ProcessOrder / ReindexSearch / GenerateDailyReport |
| `cron` + `dispatch` | DailyReport, HourlyReindex |

Known gap (hand-owned, like bookings' extra tables): the `product_tags`
join table has no DDL — the atlas backend materializes entity tables only.
`seed.EnsureJoinTable` creates it; tests call it after migrating.

## Runbook

Run from `examples/showcase/` so `data/` resolves.

Two secrets are required by every entrypoint, and the gRPC listener needs
its address:

```sh
cp .env.example .env
printf 'AUTH_JWT_SECRET=%s\nCRYPTO_KEY=%s\n' \
  "$(head -c 32 /dev/urandom | base64)" "$(head -c 32 /dev/urandom | base64)" >> .env
export GRPC_ADDR=:9090
```

`.env` holds local secrets only -- never commit it. `GRPC_ADDR` is read
from the process environment, not from `.env`.

1. Migrate: `zever db migrate --adapter=sqlite --dsn=data/showcase.db schema/shop/shop.zen`
2. Seed (idempotent): `go run ./db/seed`
3. Serve (HTTP `:8080` + gRPC `:9090`): `go run ./cmd/server`
4. Worker (jobs + embedded scheduler): `go run ./cmd/worker`

The DSN in step 1 is the file `DB_PATH` points at in `.env`, so the
migrated tables are the ones the server opens.

## CLI tour (all read-only except compile)

- `zever check schema/shop/shop.zen`
- `zever fmt schema/shop/shop.zen` (clean)
- `zever routes schema/shop/shop.zen`
- `zever graph schema/shop/shop.zen`
- `zever explain ShopService.Checkout schema/shop/shop.zen`
- `zever check-boundaries schema/shop/shop.zen`
- `zever breaking --help` (needs two schema revisions)
- `zever doctor` (with `AUTH_JWT_SECRET` and `CRYPTO_KEY` set): 32 OK,
  3 FAIL — `ai` (no API key), `geo` (no cities.json) and `i18n` (embed FS
  comes from code, like bookings). All three are batteries this app does
  not wire: expected zero-infra defaults, not bugs.
