# Showcase example

Shop 单体示例, built on the zever framework. Exercises every DSL
capability in one schema: all eleven scalars, inline + named enums,
optional fields, defaults, all four relation kinds, indexes, `@schema`,
`@renamed_from`, messages (request + nested + returns), all HTTP verbs,
every auth shape, permission checks, error sets, paginated RPCs, jobs with
params, and schedules.

## Layout

- `schema/shop/shop.zen` — the whole domain (User, Profile, Category,
  Product, Tag, Order, OrderItem, Review; ShopService; 4 jobs; 2 schedules).
- `generated/` — CLI output (`--backend=atlas,gogen,openapi,proto,protogogen,zenorm`).
  Never hand-edit; regenerate from the repo root with:
  `zever compile examples/showcase/schema/shop/shop.zen --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out examples/showcase/generated`
  (run from the root so gogen/protogogen import paths resolve).
- `internal/api/testdata/schema.sql` — DDL extracted from
  `zever db migrate --dry-run --adapter=sqlite`.
- `internal/api/` — hand-written routes (register/login, products,
  checkout, reviews) with auth, rbac, ratelimit, queue, i18n, flag.
- `internal/service/seed/` — idempotent demo data via the zenorm backend.
- `internal/service/jobs/` — one handler per declared job.
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
3. Serve: `go run ./cmd/server`
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
