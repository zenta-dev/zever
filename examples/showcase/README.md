# Showcase example

Shop example built on the zever framework. Exercises every DSL
capability in one schema: all eleven scalars, inline + named enums,
optional fields, defaults, all four relation kinds, indexes, `@schema`,
`@renamed_from`, messages (request + nested + returns), all HTTP verbs,
every auth shape, permission checks, error sets, paginated RPCs, jobs with
params, and schedules.

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
Hand-owned, never overwritten: `internal/app/app.go` (adapter selection, JWT
gate, i18n/permission runtime defaults), `internal/service/shop/shop_service.go`
(9 RPC bodies), `internal/service/jobs/*` bodies (Deps-closure = filled-in stub
form), `internal/service/seed/seed.go` bodies, `internal/api/*` rich shapes,
schema, `zever.yaml`, locales/fixtures, tests.
`db/seed/main.go` intentionally bypasses `app.New()` (`config.Load` directly)
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
- `zever.yaml` — explicit zero-infra service adapters.
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

Run from `examples/showcase/` so `zever.yaml` and `data/` resolve.
`AUTH_JWT_SECRET` (≥32 bytes) is required by every entrypoint.

1. Migrate: `zever db migrate --adapter=sqlite --dsn=data/showcase.db schema/shop/shop.zen`
2. Seed (idempotent): `go run ./db/seed`
3. Serve (HTTP `:8080` + gRPC `:9090`): `go run ./cmd/server`
4. Worker (jobs + embedded scheduler): `go run ./cmd/worker`

## CLI tour (all read-only except compile)

- `zever check schema/shop/shop.zen`
- `zever fmt schema/shop/shop.zen` (clean)
- `zever routes schema/shop/shop.zen`
- `zever graph schema/shop/shop.zen`
- `zever explain ShopService.Checkout schema/shop/shop.zen`
- `zever check-boundaries schema/shop/shop.zen`
- `zever breaking --help` (needs two schema revisions)
- `zever doctor --config zever.yaml` (needs `AUTH_JWT_SECRET` set): 32 OK,
  2 FAIL — `ai` (no API key) and `i18n` (embed FS comes from code, like
  bookings). Both are expected zero-infra defaults, not bugs.
