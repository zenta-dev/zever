# json_query — a JSON-query example app

A small, runnable documents app proving zever's typed JSON operator
helpers: one `.zen` schema, the `zenorm` codegen backend, and the sqlite
json1 helper package (`orm/json/sqlite`) building real, executing
predicates over JSON documents stored in a TEXT column.

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL with a JSON-bearing column | [`schema/json_query.zen`](schema/json_query.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| Filtering by a nested key (`json_extract(data, '$.author.name') = ?`) | [`cmd/app/main.go`](cmd/app/main.go) |
| Key existence (`json_extract(data, '$.draft') IS NOT NULL`) | [`cmd/app/main.go`](cmd/app/main.go) |
| Array membership via `json_each` (`EXISTS (SELECT 1 FROM json_each(data, '$.tags') WHERE value = ?)`) | [`cmd/app/main.go`](cmd/app/main.go) |
| Value typing via `json_type` | [`cmd/app/main.go`](cmd/app/main.go) |
| Two JSON predicates composed with `orm.And` | [`cmd/app/main.go`](cmd/app/main.go) |

Every query is an ordinary `orm.From(...).Where(...)` predicate built from
the typed helpers in `github.com/zenta-dev/zever/orm/json/sqlite` — never a
hand-written SQL string.

## Why the JSON column is `string`, not `json`

The `.zen` DSL does have a native `json` field type, and the `zenorm`
backend maps it to `json.RawMessage`. Under Go 1.27, though,
`encoding/json.RawMessage` is an alias for `encoding/jsontext.Value` — a
named `[]byte` type that implements neither `sql.Scanner` nor one of
`database/sql`'s built-in dest kinds — so the codegen'd `Scan` (which reads
`&e.Data`) fails at runtime with "unsupported Scan ... into
*jsontext.Value". Storing the JSON document in a `string` (TEXT) column
avoids that, and every json1 function (and the jsonb operators on Postgres)
operates on JSON text equally well. This is a documented known limitation
of the current `json` DSL type, not a design choice of the JSON helpers.

## Running it

From the repo root:

```bash
go run ./examples/json_query/cmd/app
```

It opens an in-memory SQLite database, seeds three JSON documents, and
prints:

```
docs authored by Grace:
  d2 (SQLite JSON1 Field Guide)
docs with a top-level "draft" key (3): d1 d2 d3
docs tagged "go" (2): d1 d3
docs whose author is a JSON object (3): d1 d2 d3
docs tagged "go" by Ada (1): d1
```

## Regenerating code from the schema

From the repo root (so import paths resolve):

```bash
zever compile examples/json_query/schema/json_query.zen --backend=zenorm --out examples/json_query/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go test -count=1 ./examples/json_query/...
```

Covers each result set above.
