# orm

Generics-typed SQL query builder over `db.DB`, with a shared render core
and capability-gated dialects (`sqlite`, `postgres`). Ported from the
`zen` package and renamed; the query builder has no mysql rendering and
there is no bundled mysql driver in `db/` (see Dialects below for where
mysql is and is not supported).

## Model

```go
type User struct {
    ID   int64
    Name string
}

func (u *User) Scan(row orm.Row) error {
    return row.Scan(&u.ID, &u.Name)
}

var (
    Users    = orm.NewTable[User]("users", []string{"id", "name"})
    UserID   = orm.NewColumn[User, int64]("users", "id")
    UserName = orm.NewColumn[User, string]("users", "name")
)

users, err := orm.From(Users).
    Where(UserName.Eq("alice")).
    OrderBy(UserID.Desc()).
    Limit(10).
    All(ctx, conn) // []*User
```

`From[T, PT]` infers the pointer-scanner type, so callers never write type
arguments. `First`/`FirstOrErr` (wraps `db.ErrNotFound`), `Count`,
`Exists`, and `Stream` (`iter.Seq2`) cover the remaining read shapes.

## Writes

`Insert`/`Update`/`Delete` builders plus `Conflict` upserts and `Merge`;
`Tx`/`WithNestedTx` run units of work with savepoints, and `RetryTx`
retries serialization failures. Statements reuse `db.Preparer` where the
adapter offers it.

## Advanced builders

Joins (`Join2/3`, outer variants), CTEs, aggregates, window functions, set
operations, projections, TVFs, keyset cursors (`CursorKey`), batched
`Preload` (no N+1), `Explain`, row locking, and per-dialect `orm/json` +
`orm/fts` helpers. Features a dialect lacks fail closed with typed
`dialect.ErrUnsupportedByDialect` (e.g. `DISTINCT ON` and strong lock modes
are postgres-only).

## Dialects

Resolved per call from `conn.Dialect()` (`"sqlite"`/`"postgres"`) — no
silent fallback. Placeholder and quoting rules plus capability gates live
in `orm/dialect`; SQL text lives in `orm/render` (shared, args always
bound, identifiers always quoted). All errors carry the `orm:` prefix
(`orm/render:`, `orm/dialect:`) and remain `errors.Is`-compatible.

Large `IN` lists: a value-list `IN` over the dialect's parameter budget is
split into several statements for plain `All`/`Stream`/`Count`/`Exists`
(repeated values are removed first so no row is returned twice). Those
statements are **not snapshot-consistent** — concurrent writes between
chunks can be missed or seen partially — unless you pass a transaction.
`ORDER BY`/`LIMIT`/`OFFSET`/locks, `NOT IN` and `IN` under `OR`/`NOT` fail
closed with `ErrInListTooLarge`.

MySQL status: the query builder above does not render mysql, and `db/`
ships no mysql driver — but the migration stack does support it.
`dsl/backend/atlas` renders bootstrap DDL for `"mysql"`, and
`orm/migrate` live-diffs a compiled schema against a MySQL database via
`information_schema` introspection (`orm/migrate/mysql.go`: backtick
quoting, `MODIFY COLUMN`, `DROP INDEX ... ON <table>`, MySQL-typed
`schema_migrations` bookkeeping). Point any `db.DB` whose `Dialect()`
returns `"mysql"` at `migrate.Plan`/`Apply`; unsupported shapes fail
closed with typed errors, and the remaining gaps (enum value-list
changes, column defaults) are documented in `orm/migrate/doc.go`.

## Debugging

Three layers, cheapest first:

```go
stmts, err := orm.From(Users).Where(UserName.Eq("alice")).DebugSQL("sqlite")
for _, s := range stmts {
    fmt.Println(s.SQL, s.Args)
}
```

`DebugSQL(dialectName)` renders exactly what `All`/`Stream` would hand
the driver -- same gates, same IN-chunk fan-out (one entry per
statement) -- without executing or needing a connection. Args stay
bound, never interpolated; they may hold secrets, so print them only in
development. `SetQueryLogger` captures every statement orm actually
executes (process-wide hook, restored via the returned func), and
`Query.Explain` / `ExplainAnalyze` return the database's own plan for
the statement (`EXPLAIN ANALYZE` is postgres-only and executes the
query).

## Testing

Units run on sqlite `:memory:` (no server). Set `POSTGRES_DSN` for a live
postgres pass:

```sh
POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' go test ./orm/...
```
