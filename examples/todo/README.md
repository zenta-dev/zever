# Todo example

Fully working todo/note CRUD app skeleton with server and tests.

Run every command from `examples/todo/` so `data/` and `schema/` resolve.

## Prereqs

- Go 1.27
- A local config file with two secrets: `AUTH_JWT_SECRET` (at least 32
  bytes) and `CRYPTO_KEY` (at least 32 bytes). Copy the template and fill
  them in:

  ```sh
  cp .env.example .env
  printf 'AUTH_JWT_SECRET=%s\nCRYPTO_KEY=%s\n' \
    "$(head -c 32 /dev/urandom | base64)" "$(head -c 32 /dev/urandom | base64)" >> .env
  ```

  `.env` holds local secrets only -- never commit it.
- `zever doctor` reports auth FAIL until `AUTH_JWT_SECRET` is set, and
  crypto FAIL until `CRYPTO_KEY` is set. Both are expected by default.
  With the two secrets set, the remaining FAILs (`ai`, `geo`, `i18n`) are
  the batteries this app does not wire -- expected zero-infra defaults.

## Schema

- Source: `schema/todo.zen`
- Validate: `zever check schema/todo.zen`
- Format check: `zever fmt -l schema/todo.zen`
- Regenerate: `zever compile schema/todo.zen --backend=zenorm,openapi,atlas --out generated`
- Preview DDL: `zever db migrate --dry-run --adapter=sqlite schema/todo.zen`
- Routes: `zever routes schema/todo.zen`

## Migrate

`zever db migrate --adapter=sqlite --dsn=data/app.db schema/todo.zen`

The DSN is the same file `DB_PATH` points at in `.env`, so the migrated
tables are the ones the server opens.

## Serve

`go run ./cmd/server -addr :8080`

## Worker

Runs the `SendOverdueDigest` job plus the `OverdueDigest` schedule (every
minute) in one process over the shared memory queue:

`go run ./cmd/worker`

## Seed

Migrate first, then seed the demo user (`demo@example.com` / `password123`)
with three notes (idempotent, safe to re-run):

`zever db migrate --adapter=sqlite --dsn=data/app.db schema/todo.zen`
`go run ./db/seed`

## Test

From the repository root: `go test -race ./examples/todo/...`
