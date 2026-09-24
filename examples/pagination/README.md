# pagination — offset and keyset pagination with orm

A small product catalog (25 products, `schema/pagination.zen`) queried two
ways: classic offset pagination (`OrderBy`/`Limit`/`Offset` + `Count`, via
`orm.OffsetPage`) and keyset (cursor) pagination driven by `orm/cursor.go`
(`orm.AfterTuple` for the multi-column position plus an opaque
`CursorKey.Encode`/`DecodeCursor` page token).

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL with a `timestamp` column (codegen'd `Scan` parses RFC3339) | [`schema/pagination.zen`](schema/pagination.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| Offset pagination: `orm.OffsetPage(q, page, size)` + `Count` for the total | [`cmd/app/main.go`](cmd/app/main.go) |
| Keyset pagination: the `(price, id) > (?, ?)` predicate built by `orm.AfterTuple` from the same typed columns every other filter uses | [`cmd/app/main.go`](cmd/app/main.go) |
| An opaque page token from `orm.NewCursorKey(...).Encode`, round-trip verified with `orm.DecodeCursor` on every hop (empty token means "first page"; replay against the wrong column fails) | [`cmd/app/main.go`](cmd/app/main.go) |
| A sort key with duplicates, so the `id` tie-break actually matters | [`cmd/app/main.go`](cmd/app/main.go) |

The keyset predicate must mirror the `OrderBy` exactly — here
`(price_cents ASC, id ASC)` — or pages can skip or duplicate rows. Unlike
OFFSET, the keyset page's `WHERE` selects the window itself, so a page stays
correct when rows are inserted or deleted between loads.

## Running it

```bash
go run ./examples/pagination/cmd/app
```

It opens an in-memory SQLite database, seeds 25 products, prints offset
page 2 (with the total count), then walks every page via opaque keyset
cursors. Both styles return the same page contents:

```
offset page 2 of 25 total products (LIMIT 5 OFFSET 5):
  p15 gadget-15     7.49
  p22 gadget-22     7.49
  p02 gadget-02     9.99
  p09 gadget-09     9.99
  p16 gadget-16     9.99
keyset walk (page size 5, ascending price then id):
  page 1 (cursor ): p07:499 p14:499 p21:499 p01:749 p08:749
  page 2 (cursor AQAIcHJvZHVjdHMCaWRzA3AwOA): p15:749 p22:749 p02:999 p09:999 p16:999
  page 3 (cursor AQAIcHJvZHVjdHMCaWRzA3AxNg): p23:999 p03:1249 p10:1249 p17:1249 p24:1249
  page 4 (cursor AQAIcHJvZHVjdHMCaWRzA3AyNA): p04:1499 p11:1499 p18:1499 p25:1499 p05:1749
  page 5 (cursor AQAIcHJvZHVjdHMCaWRzA3AwNQ): p12:1749 p19:1749 p06:1999 p13:1999 p20:1999
```

## Regenerating code from the schema

From the repo root:

```bash
zever compile examples/pagination/schema/pagination.zen --backend=zenorm --out examples/pagination/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go build ./examples/pagination/...
go test -count=1 ./examples/pagination/...
```
