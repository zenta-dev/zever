# Multidb example

Three sqlite databases in one process: the default instance plus two named
ones (`analytics`, `audit`), each addressed by its own env key.

Run every command from `examples/multidb/` so `data/` resolves.

## Layout

- `internal/app/wiring.go` — the three `db.Ref` vars (`DB`, `DBAnalytics`,
  `DBAudit`) and one `container.Provide` entry per instance. A named
  instance is addressed by `DB__<NAME>__<FIELD>` env keys.
- `cmd/server/main.go` — the entrypoint; `GET /healthz` pings all three
  databases.
- `.env.example` — every service env key with its default.

## Prereqs

The local crypto adapter refuses the deterministic dev key outside a dev
`ZEVER_ENV`, so a real `CRYPTO_KEY` belongs in `.env` (never commit it):

```sh
cp .env.example .env
printf 'CRYPTO_KEY=%s\n' "$(head -c 32 /dev/urandom | base64)" >> .env
```

## Databases

This example ships no schema of its own -- it demonstrates the wiring, not
a domain. The three databases are separate files:

| Instance | Env key | File |
|---|---|---|
| default | `DB_PATH` | `data/multidb.db` |
| analytics | `DB__ANALYTICS__PATH` | `data/analytics.db` |
| audit | `DB__AUDIT__PATH` | `data/audit.db` |

Migrate each one with your own schema, for example
`zever db migrate --adapter=sqlite --dsn=data/multidb.db schema/app.zen`.

## Serve

```sh
go run ./cmd/server
curl localhost:8080/healthz
```

`GET /healthz` answers 200 once all three databases answer a ping.

## Doctor note

`zever doctor` checks every battery the framework knows, so the three this
app does not wire (`ai`, `geo`, `i18n`) report FAIL -- expected zero-infra
defaults, not bugs. The `db` row needs the three files above to exist.
