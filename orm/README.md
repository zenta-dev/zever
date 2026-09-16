# orm

Generics-typed SQL query builder over `db.DB`, with a shared render core
and capability-gated dialects (`sqlite`, `postgres`). Ported from the
`zen` package and renamed; mysql support was dropped (no mysql adapter in
`db/`).

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

## Testing

Units run on sqlite `:memory:` (no server). Set `POSTGRES_DSN` for a live
postgres pass:

```sh
POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' go test ./orm/...
```
