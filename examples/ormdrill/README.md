# ormdrill — raw-orm drill, no codegen

A widget-shop flow (`widgets` → `orders` → `shipments`, plus a
`widgets_archive` copy and a `promotions` table) that exercises the advanced
`orm` query-builder surface end to end on SQLite.

Unlike the other examples it has **no `.zen` schema and no codegen step**:
the model is wired by hand with the exact constructors the `zenorm` backend
would have generated (`orm.NewTable` / `orm.NewColumn` /
`orm.NewNullableColumn` / `orm.NewRelation`) plus hand-written
pointer-receiver `Scan` methods — so it is the reference for building
against `orm` without the schema DSL at all.

`created_at` is stored as RFC3339 TEXT and parsed by each entity's `Scan`;
`note` is the nullable column backing the `COALESCE`/`NULLS`-ordering flows.

## Running it

```bash
go run ./examples/ormdrill/cmd/app
```

It opens an in-memory SQLite database, seeds 8 widgets / 15 orders /
7 shipments (plus a partial-index promotion and a widget archive), then
prints each section. The same sections run as `Example` tests:

```bash
go test -count=1 ./examples/ormdrill/
```

## Layout

- `model.go` — hand-wired tables, columns, relations, `OpenShop`/`SeedShop`.
- `querylog.go` — `SetQueryLogger` debug hook.
- `cursor.go` — typed keyset cursors (`CursorKey`/`NextPage`/`DecodeCursor`/`OffsetPage`).
- `basics.go` — `Preload`, `RetryTx`/`IsRetryable`, `FirstOrErr`.
- `join.go` — `JoinOn3`/`WhereRight`/`WhereC`, `RightJoinOn`, `Update.Join`.
- `subquery.go` — `InSub`/`NotInSub`, `Exists`/`NotExists`, `EqScalar`, correlated `Outer` refs, `Tuple.In`.
- `expr.go` — `Coalesce`/`NullIf`/`Lower`/`Upper`/`Trim`/`Length`/`Case`, `Expr.As`/`Project`/`ScalarSubquery`, `NullsFirst`/`NullsLast`.
- `mutate_gate.go` — `Insert.Select`, `Distinct`, locking gates, `DistinctOn`/extended locks/`Tablesample` gates, mutation `ORDER BY`/`LIMIT` gate.
- `returning_upsert.go` — `Update`/`Delete` `Returning` + `ExecReturning`, `OnConflict(...).Where(...).DoUpdate(...).Where(...)`.
- `join3outer.go` — `LeftJoinOn3`, `InnerLeftJoinOn3`, `LeftInnerJoinOn3`, `MixedJoinOn3`, `RightJoinOn3`, `FullJoinOn3`.
- `cmd/app/main.go` — thin runner printing all topics in dependency order.

The capability gates are documented here rather than guessable from the SQL:
`RightJoinOn3`/`FullJoinOn3` hops below SQLite 3.39, `ForUpdate`/`ForShare`/
`SkipLocked` on SQLite, `DistinctOn`/extended locks/`Tablesample` outside
Postgres, and single-table mutation `ORDER BY`/`LIMIT` outside MySQL all
return the typed `dialect.ErrUnsupportedByDialect`; the invalid locking
combinations return the typed `orm.ErrLocking*` errors.

## Not yet in zever/orm

Nothing skipped: every topic of the reference predecessor drill
(`Personal/zen-go/examples/enhancements`, rounds 1–8) maps to an existing
`zever/orm` API, including the `JoinOn3` variants, `Tuple.In`,
`NullsFirst`/`Last`, `OnConflict.Where`, `Returning`, `Insert.Select`,
`Distinct`, and `Expr.As`/`Project`/`ScalarSubquery`.
