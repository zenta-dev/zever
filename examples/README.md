# Examples

Runnable examples leveraging every `zever` service battery, plus one
showcase app that wires them together.

## Layout

```text
examples/
  README.md          # this file: map, runbook, conventions
  <battery>/         # STAGE 1: one focused Example test per battery
  bookings/          # STAGE 2: showcase marketplace app (server + worker)
```

There is deliberately **no new Go module** here. Examples live in the main
module so `go test ./examples/...` covers them and `internal/` packages are
importable. If an example ever needs a heavy extra dependency, isolate just
that one behind its own `go.mod`.

## Stage 1: per-battery examples

One `ExampleXxx` test per battery using its zero-infra adapter. Each example
shows one happy path and one error path via `// Output:` comments, so the
test suite itself is the documentation. Planned batches:

- Batch 1: `auth`, `cache`, `queue`, `db` (+ `orm`), `router`, `log`,
  `config`, `container`
- Batch 2: `payment`, `billing`, `geo`, `media`, `search`, `vectorstore`,
  `i18n`, `flag`
- Batch 3: `eventbus`, `webhook`, `notification`, `mailer`, `job`,
  `scheduler`, `workflow`, `idempotency`, `lock`, `session`, `tenant`,
  `authz`, `permission`
- Batch 4: `ai`, `analytics`, `crypto`, `password`, `secrets`, `storage`,
  `document`, `observability`, `codec`

Zero-infra adapters used (all from `config.Default()`):

| Battery | Adapter | Battery | Adapter |
|---|---|---|---|
| ai | anthropic (needs key, see below) | lock | memory |
| analytics | log | log | slog |
| auth | jwt (needs secret, see below) | mailer | log |
| authz | rbac rules (no registry) | media | local |
| billing | stub | notification | log |
| cache | memory | observability | stdout |
| codec | json (no registry) | password | argon2id |
| config | file + env | payment | stub |
| container | New(nil) | permission | noop/rbac |
| crypto | local | queue | memory |
| db | sqlite | ratelimit | memory |
| document | local | router | stdhttp |
| eventbus | memory | scheduler | embedded |
| flag | static | search | sqlite |
| geo | static (needs data, see below) | secrets | env |
| i18n | embed (needs FS, see below) | session | memory |
| idempotency | memory | storage | local |
| job | memory queue | tenant | single/header |
| vectorstore | sqlite | webhook | http + queue |
| workflow | memory | orm | sqlite via db |

## Stage 2: bookings showcase

Domain: bookings marketplace ("stayver"). Hosts list spaces, guests book
them, the platform takes stub payments and sends notifications. Chosen over a
`todo` port because only a rich domain reaches `payment`, `billing`, `geo`,
`media`, `search` and `vectorstore`.

Flows:

1. Host lists space: `auth` JWT issue, `permission` rbac check, `media`
   upload, `geo` geocode, `db` insert via `orm`, `search` index,
   `vectorstore` upsert, `eventbus` publish `space.listed`.
2. Guest books: `ratelimit` allow, `idempotency` reserve-or-replay, `lock`
   hold dates, `payment` stub charge, `billing` host subscription, `queue`
   confirmation job, `mailer` + `notification` send, `webhook` deliver
   `booking.created`, `scheduler` reminder.
3. Platform: `tenant` resolve, `session` login, `i18n` confirmation,
   `flag` gate checkout, `analytics` track, `observability` traces,
   `crypto` PII field, `password` credentials, `secrets` webhook secret,
   `storage` receipt, `document` invoice, `config` + `container` wiring.

`cmd/server` and `cmd/worker` mirror `zever generate server` / `worker`
output shapes so the generator stays honest; drift found becomes follow-up
issues, not drive-by fixes.

## Running

```sh
go test ./examples/...            # everything, zero infra
go test -race ./examples/...     # race detector
```

Zero-infra defaults need nothing else. Infra adapters are opt-in via env
(documented per example when added):

```sh
# Q_ADAPTER=redis REDIS_ADDR=localhost:6379 go test ./examples/queue/...
# DB_ADAPTER=postgres DB_DSN=... go test ./examples/db/...
```

## Known gaps (found via `zever doctor`, not fixed here)

`zever doctor` on `config.Default()` reports 30 OK, 4 FAIL:

- `ai` (anthropic): `api_key is required`. Default picks a keyed adapter
  with no key path. Example tracks: use httptest server or document the
  env requirement.
- `auth` (jwt): `jwt secret is required`. Same shape: keyed default, no
  dev secret. Example must supply one explicitly (never commit it).
- `geo` (static): `cities.json: no such file or directory`. Static data
  file missing from repo. Example must ship a tiny fixture or use `osm`
  via httptest.
- `i18n` (embed): `embed FS is required`. Host app must provide the FS.
  Example embeds its own locale files.

Plus two recently-wired batteries to prefer via the container: `lock`
(memory/redis) and `secrets` (env) both have `config` sections and
`container` accessors; examples should resolve them through
`container.New(nil)` like every other battery.

## Conventions for new examples

- Deterministic: no `time.Sleep` sync, no network, no unseeded randomness,
  no timing assertions (repo testing standard).
- Scheduler/job examples drive ticks manually, never wall-clock.
- Never commit secrets or keys; test fixtures stay in `testdata/`.
- Doc comment on the example function stating which battery and adapter it
  shows.
