# Todo example

Fully working todo/note CRUD app skeleton with server and tests.

## Prereqs

- Go 1.27
- JWT secret in the environment only (never in `zever.yaml`):
  `export AUTH_JWT_SECRET='<at-least-32-bytes>'`
- `zever doctor` reports auth FAIL until the secret is set; that is expected by default.

## Schema

- Source: `schema/todo.zen`
- Validate: `/tmp/opencode/zever check schema/todo.zen`
- Format check: `/tmp/opencode/zever fmt -l schema/todo.zen`
- Regenerate: `/tmp/opencode/zever compile schema/todo.zen --backend=zenorm,openapi,atlas --out generated`
- Preview DDL: `/tmp/opencode/zever db migrate --dry-run --adapter=sqlite schema/todo.zen`
- Routes: `/tmp/opencode/zever routes schema/todo.zen`

## Migrate

`/tmp/opencode/zever db migrate --adapter=sqlite --dsn=data/app.db schema/todo.zen`

## Serve

`go run ./examples/todo/cmd/server -addr :8080`

## Worker

Runs the `SendOverdueDigest` job plus the `OverdueDigest` schedule (every
minute) in one process over the shared memory queue:

`go run ./examples/todo/cmd/worker`

## Seed

Migrate first, then seed the demo user (`demo@example.com` / `password123`)
with three notes (idempotent, safe to re-run):

`/tmp/opencode/zever db migrate --adapter=sqlite --dsn=data/app.db schema/todo.zen`
`go run ./examples/todo/db/seed`

## Test

`go test -race ./examples/todo/...`
