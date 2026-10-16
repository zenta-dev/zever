# Threat model: db

Audited against `core/db/` (`db.go`, `options.go`, `tx.go`) and
`adapters/db/postgres`, `adapters/db/sqlite` at WS6 time.

## Assets

- The DSN (credentials in cleartext in the connection string) and every row
  the process can read.
- The shared connection pool: exhaustion or misuse is a process-wide DoS.

## Trust boundaries

- SQL text crosses from `orm` (typed builder) or caller-supplied migrations
  into the driver. `core/db` itself takes raw `query string` + args: it is
  a facade, not an injection barrier.
- The DSN crosses config → adapter → driver. Pool settings (`MaxConns`,
  `MinConns`, lifetimes) cross config → pool.

## What zever guarantees

- `Options.Validate` range-checks pool and timeout fields (no negatives);
  adapters own required-field checks (DSN/Path presence).
- `orm` (the normal query path) always binds args as placeholders and always
  quotes identifiers; per-dialect capability gates fail closed with
  `dialect.ErrUnsupportedByDialect` instead of emitting wrong SQL.
- Migration lint (`orm/migrate`, `zever db migrate --lint`) fails on
  error-severity findings (`DROP TABLE`/`DROP COLUMN`, `NOT NULL` without
  `DEFAULT`, type rewrites) unless waived per statement; postgres applies
  opt-in `lock_timeout`/`statement_timeout` via `SET LOCAL` per statement.
- `Preparer` documents the cheap-reuse contract callers rely on
  (`core/db/db.go`).

## What the caller must do

- Never interpolate values into SQL text: use `orm` or placeholders. Raw
  `db.Query(ctx, query, args...)` with string-built `query` is an injection
  hole zever cannot see.
- Keep the DSN in secrets management, never in logs (use redacted display);
  connect with a least-privilege DB role; require TLS to the database in
  production (see `container` sslmode handling for postgres).
- Size pools deliberately; use `dedicated_pool: true` when one service must
  not starve another. Run migrations with `--lint` and timeouts in CI.
- Validate stored identifiers on rollback replay (`orm/migrate` guards,
  fix/s2 workstream).

## Explicit non-guarantees

- `core/db` does not parse or sanitize SQL text. Injection safety comes from
  the `orm` layer discipline, not from the facade.
- SQLite file permissions are the operator's job: a world-readable sqlite
  file leaks the whole database regardless of any framework guarantee.
