# fts — a full-text-search example app

A small, runnable search app proving zever's typed full-text-search
helpers: one `.zen` schema, the `zenorm` codegen backend, an FTS5 virtual
table, and queries built through the `orm/fts/sqlite` package's
`Match`/`Rank` helpers — no raw SQL at the call site.

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL with a free-text `body` column | [`schema/fts.zen`](schema/fts.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| FTS5 virtual-table creation via raw DDL (the FTS5 shape is a migration concern, not expressible in the `.zen` DSL) | [`cmd/app/main.go`](cmd/app/main.go) |
| `orm/fts/sqlite.Match` — an FTS5 `col MATCH ?` predicate composing into `orm.From(...).Where(...)` | [`cmd/app/main.go`](cmd/app/main.go) |
| `orm/fts/sqlite.Rank` — `ORDER BY rank` over FTS5's hidden relevance column | [`cmd/app/main.go`](cmd/app/main.go) |
| `Count` over the same MATCH predicate | [`cmd/app/main.go`](cmd/app/main.go) |
| `orm.Insert` / `orm.Set` for the seed data (mutations share the same typed columns) | [`cmd/app/main.go`](cmd/app/main.go) |

Known limitation: the FTS5 virtual table (`CREATE VIRTUAL TABLE docs
USING fts5(...)`) is created via raw `conn.Exec` in `seed` because the
`.zen` DSL cannot express virtual-table shapes — it is a migration concern,
not an entity concern.

## Running it

From the repo root:

```bash
go run ./examples/fts/cmd/app
```

It opens an in-memory SQLite database, creates the FTS5 virtual table,
seeds four documents, and prints:

```
body matches "go": [d1 "Go release" d4 "Go on the web"]
body matches "go", most relevant first:
  d1 "Go release"
  d4 "Go on the web"
body matches "go": count = 2
```

The query text is always a bound placeholder — FTS5's native
boolean/phrase syntax (terms, `AND`/`OR`/`NOT`, `"phrases"`) flows through
the argument, never into the SQL text.

## The Postgres twin (not run)

`orm/fts/postgres` exposes the same surface over Postgres `tsvector`
machinery (`to_tsvector('english', ...) @@ plainto_tsquery(...)`,
`ts_rank` ordering). This demo runs sqlite only; the postgres variant needs
a live Postgres and is not exercised here.

## Regenerating code from the schema

From the repo root (so import paths resolve):

```bash
zever compile examples/fts/schema/fts.zen --backend=zenorm --out examples/fts/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go test -count=1 ./examples/fts/...
```

Covers the match set, the rank order, and the count.
