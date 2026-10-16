# How zever compares

zever is a schema-driven scaffolding compiler for Go: one `.zen` schema
drives a typed ORM, SQL migrations, OpenAPI, and routes, with every backend
concern behind a small swappable-adapter interface. This page states
plainly where common alternatives are the better pick.

Related: [STABILITY.md](../STABILITY.md) (Core vs Extended tiers),
[config table](../config/README.md), [container](../container/README.md),
[CLI map](../cmd/zever/README.md).

## sqlc

sqlc generates type-safe queries from SQL you already wrote. It is the
better pick when you already own your schema SQL, want the thinnest
possible codegen layer, and do not want to adopt a framework, a DSL, a
container, or layered config to get it.

Pick zever instead when you want the schema to be the source of truth going
the other direction: declare entities once in `.zen` and derive the
migration SQL, the typed query builder, and the API surface from it.

## ent

ent models rich entity graphs in Go with built-in codegen for privacy
rules, hooks, and versioned migrations. It is the better pick when your
domain is a complex graph and you want those semantics expressed in Go
with a mature generator behind them.

Pick zever instead when your priority is swapping infrastructure by config
(sqlite to Postgres, memory to Redis) and keeping generated output to
plumbing — zever never generates business logic.

## Goa

Goa designs APIs first: a design language produces servers, clients, and
documentation with a long-matured pipeline. It is the better pick when API
design is the center of your project and persistence is secondary or
handled elsewhere.

Pick zever instead when the schema must also own persistence — the same
`.zen` file that shapes the API emits the ORM types and the migration
HCL/SQL.

## Encore

Encore pairs a framework with hosting: deploys, preview environments, and
observability out of the box. It is the better pick when you want a
platform to run your code, not just generate it.

Pick zever instead when you want generated code you own and run anywhere.
`zever serve` and `zever dev` run locally; there is no cloud platform, no
deploy target, no managed control plane.

## Plain chi + sqlc, hand-wired

A hand-wired stack is the better pick for small services: no DSL to learn,
no container to reason about, full control over every line, and nothing to
upgrade. If the service fits in a few files, framework surface costs more
than it saves.

Pick zever instead when copy-pasted wiring is the bottleneck: one schema
change regenerating the ORM, migrations, OpenAPI, and routes consistently,
with zero-infra defaults (sqlite, in-memory, local) so tests and first
runs need no Docker.

## Trying the zever side

```bash
zever new hello --dir ./hello --force
cd hello
zever check schema/app.zen
zever compile --backend=zenorm,atlas,openapi --out ./generated schema/app.zen
zever db migrate --dry-run --adapter=sqlite schema/app.zen
```

See the [README](../README.md#quickstart-schema-to-database-in-2-minutes)
for the schema snippet and the exact Go the `zenorm` backend emits for it.
