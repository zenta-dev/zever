# Threat model: orm

Audited against `orm/` (`render/`, `dialect/`, `migrate/`, `cursor.go`) at
WS6 time.

## Assets

- Query correctness as a security property: wrong SQL (wrong dialect quoting,
  silently dropped clauses) becomes data corruption or access-control bypass
  in row-level-security deployments.

## Trust boundaries

- Model metadata (`NewTable`/`NewColumn`, struct shapes) is developer code
  and therefore trusted; query *values* are untrusted and must never reach
  SQL text.
- Dialect is resolved per call from `conn.Dialect()`: the connection, not a
  global, decides the SQL flavor. There is no silent fallback.

## What zever guarantees

- Rendered SQL text lives in `orm/render`; args are always placeholder-bound
  and identifiers always quoted. No raw-SQL string building is offered by
  the typed API.
- Features a dialect lacks fail closed with `dialect.ErrUnsupportedByDialect`
  (e.g. `DISTINCT ON`, strong lock modes are postgres-only); they never
  silently emit wrong SQL.
- Oversized value-list `IN`s are deduped (order-preserving, NULLs collapsed)
  and auto-chunked per the dialect parameter budget (postgres 65535, sqlite
  999/32766 version-gated); chunking refuses non-reconstructible shapes
  (`ORDER BY`/`LIMIT`/`OFFSET`, locks, `TABLESAMPLE`, `OR`/`NOT` nesting)
  with `ErrInListTooLarge` instead of guessing.
- All errors carry an `orm:`-family prefix and stay `errors.Is`-compatible;
  cursor tags are validated before `int32` conversion (`orm/cursor.go`).

## What the caller must do

- Chunked reads are multiple statements and are **not snapshot-consistent**
  unless run in a transaction: wrap consistency-sensitive multi-chunk reads
  (and preloads) in a tx.
- Use `zever db migrate --lint` in CI; waive rules per statement with
  `-- zever:allow:<code>` only with a recorded reason.
- Treat `orm` model tags and migration files as code: review them like code,
  especially rollback replays.

## Explicit non-guarantees

- `orm` is not an authorization layer. Table access control, RLS policies,
  and tenant scoping are caller-owned; the builder happily renders a query
  the caller should not have asked.
