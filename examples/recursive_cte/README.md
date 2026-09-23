# recursive_cte — a recursive CTE example app

A small, runnable org-chart app proving zever's `WITH RECURSIVE` support:
one `.zen` schema, the `zenorm` codegen backend, and a single
`orm.WithRecursive` query that walks the whole tree — no N+1 preloads.

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL with a self-referential relation (`belongs_to manager`, `has_many reports`, index on `manager_id`) | [`schema/recursive_cte.zen`](schema/recursive_cte.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| `orm.WithRecursive` — one recursive CTE returning every employee reachable from the root | [`cmd/app/main.go`](cmd/app/main.go) |
| Recursive CTE `Count` and a subtree query filtered by the outer WHERE | [`cmd/app/main.go`](cmd/app/main.go) |
| `orm.Insert` / `orm.Set` for the seed data (mutations share the same typed columns) | [`cmd/app/main.go`](cmd/app/main.go) |
| Safe CTE identifiers and join keys (validated `orm.NewCTEName`, keys from codegen'd `Col().Name()`) | [`cmd/app/main.go`](cmd/app/main.go) |

## Running it

From the repo root:

```bash
go run ./examples/recursive_cte/cmd/app
```

It opens an in-memory SQLite database, seeds a seven-person org chart, and
prints:

```
org chart (7 employees via one WITH RECURSIVE query):
Ada CEO (Chief Executive Officer)
  Grace VP (VP Engineering)
    Margaret Lead (Engineering Lead)
      Barbara Eng (Engineer)
      Radia Eng (Engineer)
  Katherine VP (VP Sales)
    Alan Rep (Sales Representative)
total headcount via recursive CTE count: 7
Grace VP's org (recursive subtree): Barbara Eng Grace VP Margaret Lead Radia Eng
```

## Regenerating code from the schema

From the repo root (so import paths resolve):

```bash
zever compile examples/recursive_cte/schema/recursive_cte.zen --backend=zenorm --out examples/recursive_cte/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go test -count=1 ./examples/recursive_cte/...
```

Covers the tree order, the headcount (`7`), and the subtree query.
