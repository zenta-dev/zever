# db

Adapter registry for SQL databases (`sqlite`, `postgres`). Application code
speaks the `db.DB` interface with `?` placeholders on both drivers; the
postgres driver rewrites them to `$n` before handing the query to pgx.

## Adapters

| Adapter | Backend | DSN |
|---|---|---|
| `sqlite` | `modernc.org/sqlite` via `database/sql` | `Options.Path` file path; empty defaults to `app.db`; `:memory:` for tests |
| `postgres` | `pgxpool` | `Options.DSN` postgres URL (required) |

```go
_ = db.Register(db.SQLite, sqlite.New) // once, at startup

conn, err := db.Open(db.SQLite, db.Options{Path: ":memory:"})
```

`MinConns` semantics differ by backend: postgres treats it as the pool
minimum; sqlite applies it as `SetMaxIdleConns` (database/sql has no
minimum-pool concept). `:memory:` sqlite forces a single connection even if
`MaxConns` is set — extra connections to an in-memory database would each
get an isolated database.

## Transactions

```go
err := db.WithTx(ctx, conn, &db.TxOptions{Isolation: db.Serializable}, func(ctx context.Context, tx db.Tx) error {
    _, err := tx.Exec(ctx, "INSERT INTO users (name) VALUES (?)", "alice")
    return err
})
```

`WithTx` begins, commits on success, rolls back on error or panic
(re-panicking after rollback), and rejects nesting (`ErrNestedTx`) and
non-transactional backends (`ErrTxUnsupported`). `Tx` embeds `DB`, so
queries run unchanged inside or outside a transaction; `Savepoint` /
`RollbackTo` take validated `[_A-Za-z][_A-Za-z0-9]*` names (interpolated
identifiers, never user values).

## Statements

Backends that cache server-side preparation implement `db.Preparer`
(sqlite does, over an internal LRU; postgres relies on pgx's per-connection
statement cache and does not). Prepared statements are reusable across calls
with different arguments.

## Testing

sqlite is fully testable in-process via `:memory:`. postgres unit tests run
without a server (config parsing, placeholder rewriting, fail-fast error
paths, interface fakes); set `POSTGRES_DSN` for the live integration test:

```sh
POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' go test ./core/db/...
```
