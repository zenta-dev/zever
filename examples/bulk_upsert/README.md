# bulk_upsert — multi-row inserts, upserts and RETURNING with orm

A widget inventory table (`schema/bulk_upsert.zen`) written in bulk through
the `orm.Insert` builder: chained `Values(...)` calls build one multi-row
INSERT, `OnConflict(id).DoUpdate(...)` turns it into an upsert, and
`Returning()` hands every written row back through the codegen'd `Scan`
method — all in a single statement. A partial unique index plus
`OnConflict(name).Where(...)` then shows the conflict-target predicate:
only rows covered by the partial index arbitrate the upsert.

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL with a `uuid` primary key (the conflict target) | [`schema/bulk_upsert.zen`](schema/bulk_upsert.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| Multi-row INSERT: chained `Values(...)` calls render one `INSERT ... VALUES (...), (...), (...)` | [`cmd/app/main.go`](cmd/app/main.go) |
| `OnConflict(col).DoUpdate(...)` — colliding rows are updated in place, new rows inserted | [`cmd/app/main.go`](cmd/app/main.go) |
| `Returning().ExecReturning(...)` — every written row scanned back via the codegen'd `Scan`, in one statement | [`cmd/app/main.go`](cmd/app/main.go) |
| `OnConflict(col).DoNothing()` — the conflict branch produces no RETURNING row | [`cmd/app/main.go`](cmd/app/main.go) |
| `OnConflict(col).Where(...)` — the conflict-target predicate names a partial unique index: names inside it update, names outside it insert | [`cmd/app/main.go`](cmd/app/main.go) |
| `UNIQUE` on the conflict target (DDL), since `ON CONFLICT` needs a unique/primary-key collision target | [`cmd/app/main.go`](cmd/app/main.go) |

Two semantics worth knowing:

- `DoUpdate`'s assignments are **literals applied uniformly to every
  conflicting row** — orm has no Postgres `EXCLUDED` pseudo-row reference,
  so the demo's upsert is a "reconcile the whole batch" shape: each colliding
  row's price/stock are reset to the batch's values. For per-row update
  values, write the rows with `Update` instead.
- **Plain INSERTs do not render `RETURNING`** — it only takes effect on the
  `OnConflict` path, so the demo reads the bulk-inserted rows back with an
  ordinary query and reserves `Returning().ExecReturning` for the upsert.
- The target `Where` predicate must match the partial index's own `WHERE`
  clause structurally, so it is spelled as literal SQL through
  `orm.UnsafeRaw` (e.g. `UnsafeRaw[Widget]("stock > 0")`), not a
  parameter-binding column comparison.

## Running it

```bash
go run ./examples/bulk_upsert/cmd/app
```

It opens an in-memory SQLite database, seeds two widgets, and prints:

```
bulk INSERT of 3 new rows (one statement, read back):
  w3 sprocket      9.90 stock=25
  w4 cog           3.10 stock=60
  w5 ratchet      21.00 stock=5
bulk upsert of 4 rows (2 existing updated, 2 inserted), RETURNING all of them:
  w1 gizmo        12.34 stock=77
  w2 doohickey    12.34 stock=77
  w6 thimble       1.80 stock=200
  w7 widget-7      8.00 stock=8
re-insert of w1 with ON CONFLICT DO NOTHING: 0 RETURNING rows (the conflict produced none)
upsert of "gadget-x" with ON CONFLICT (name) WHERE stock > 0 (matched the partial index, updated w8):
  w8 gadget-x      7.77 stock=33
upsert of "dormant" with ON CONFLICT (name) WHERE stock > 0 (outside the partial index, inserted w11):
  w11 dormant       1.00 stock=1
final inventory (10 widgets):
  w1 gizmo        12.34 stock=77
  w10 dormant       6.00 stock=0
  w11 dormant       1.00 stock=1
  w2 doohickey    12.34 stock=77
  w3 sprocket      9.90 stock=25
  w4 cog           3.10 stock=60
  w5 ratchet      21.00 stock=5
  w6 thimble       1.80 stock=200
  w7 widget-7      8.00 stock=8
  w8 gadget-x      7.77 stock=33
```

## Regenerating code from the schema

From the repo root:

```bash
go run ./cmd/zever compile examples/bulk_upsert/schema/bulk_upsert.zen --backend=zenorm --out examples/bulk_upsert/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go build ./examples/bulk_upsert/...
go test -count=1 ./examples/bulk_upsert/...
```
