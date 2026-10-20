# config

Env-only service configuration: zero-infrastructure defaults, overlaid by
environment variables. Later layers win:
`defaults < .env < .env.local < process env`.

There are no config files for services. A legacy `zever.yaml`/`.yml`/`.json`
in the working directory is ignored (with a one-line stderr warning); see
[Migration from zever.yaml](#migration-from-zeveryaml) below.

## Precedence

1. **Default** — `Default()` returns zero-infrastructure adapters (memory,
   local, sqlite, log, noop, ...; a few batteries default to the `db`
   adapter instead, e.g. `outbox`, `search`, `vectorstore`) so tests run
   without manual setup. The
   only dev secret is the crypto key, which `Validate` requires; it is
   deterministic and must never ship to production.
2. **Env** — `FromEnv()` reads `.env`, then `.env.local` (both optional),
   then overlays the real process environment, and `Resolve(env)` applies
   the merged map over the defaults and validates fail-closed. `LoadEnv`
   parses explicit dotenv files; `ApplyEnv` applies one service+instance;
   `DefaultOptions` returns fresh per-service defaults; `InstanceNames`
   lists configured named instances.

## Service table

| Service       | Adapters                            |
|---------------|-------------------------------------|
| ai            | anthropic, openai, gemini, ollama    |
| analytics     | log, posthog                        |
| auth          | jwt, session, oidc                  |
| billing       | stub, stripe, paddle                |
| cache         | memory, redis, db                   |
| cdn           | noop, cloudflare                    |
| crypto        | local, kms                          |
| db            | sqlite, postgres                    |
| document      | local, remote, latex                |
| eventbus      | memory, redis                       |
| flag          | static, firebase                    |
| geo           | google, static, osm                 |
| i18n          | embed, remote                       |
| idempotency   | memory, redis, db              |
| lock          | memory, redis                       |
| log           | noop, zerolog, slog, pretty         |
| mailer        | log, smtp                           |
| media         | local, s3                           |
| notification  | log, twilio, fcm                    |
| observability | noop, stdout, otlp                  |
| outbox        | memory, db, cdc                     |
| password      | argon2id                            |
| payment       | stub, stripe, paddle                |
| permission    | noop, rbac, casbin                  |
| queue         | memory, redis, db                   |
| ratelimit     | memory, redis                       |
| resilience    | memory, redis                       |
| router        | fiber, stdhttp                      |
| scheduler     | embedded, postgres                  |
| search        | db, postgres, meilisearch, sqlite   |
| secrets       | env, vault                          |
| session       | memory, redis, db                   |
| storage       | local, s3, r2                       |
| tenant        | single, header                      |
| vectorstore   | db, sqlite, pgvector, qdrant        |
| webhook       | http, queue                         |
| workflow      | memory, db, postgres                |

Two shipped adapters have no registry/config default: `session/cookie`
and `media/ffmpeg` exist as adapter packages but are absent from the
default adapter matrix above (and from `config.Default()`).

Defaults pick the zero-infra adapter per service (ai has none — all backends
need keys — so it defaults to `anthropic` and requires an API key via
env; mailer defaults to `log` with a dummy localhost SMTP host/port
because its `Validate` requires them; secrets defaults to `env` with a
`ZEVER` prefix because its `Validate` requires a non-empty prefix — that
prefix scopes the secret *names* the secrets service exposes, not service
configuration itself).

## Env scheme

`<SERVICE>_<FIELD>=value`, uppercased, no prefix:

```sh
DB_ADAPTER=postgres
DB_MAXCONNS=20
DB_MAXCONNLIFETIME=5m
AUTH_JWT_SECRET=prod-secret-at-least-32-bytes-long
RATELIMIT_RATE=100
```

Field matching is case-insensitive with `_` ignored (`MAX_RETRIES` matches
`MaxRetries`). One nesting level is supported by splitting at the first `_`
(`AUTH_JWT_SECRET` sets `Options.JWT.Secret`). Durations take `"5s"` style
strings. `_ADAPTER` sets the service adapter (validated later by
`Validate`, not at assignment).

Named instances add a middle segment: `DB__REPORTING__DSN` sets the DSN of
the `reporting` db instance. Instance names normalize case-insensitively;
a flat field and a nested path matching one key is a collision and fails.
`InstanceNames(env, "db")` lists the configured instance names
(uppercased, sorted).

### Ambient-environment safety and the typo rule

The env namespace is flat (`<SERVICE>_<FIELD>`), so it shares the process
environment with every other program on the machine. A key under a wired
service prefix therefore has three outcomes:

| Key | Outcome |
|---|---|
| Resolves to a settable field | Applied. |
| Resolves to no field but is close to one (a typo) | Fails with an `UnknownFieldError` and a did-you-mean hint (`DB_MAXCONN` → `did you mean "max_conns"?`). |
| Resolves to no field and is close to none | Ignored: an unrelated ambient variable (`AI_AGENT`, `LOG_LEVEL`, `DB_URL`, `CACHE_DIR`). It lands on no field. |

"Close" is the existing suggestion machinery (`closest` in `suggest.go`):
edit distance ≤ 2, or ≤ 3 for names longer than six characters. That
threshold is what separates a typo from an ambient variable: `AUTH_JWT_SECRT`
is one edit away from `secret`, while `AI_AGENT` is nowhere near any `ai`
field.

Unknown heads (`PATH`, `HOME`, `FOO_BAR`), bare names (`DB`), and keys of
other services are ignored in every mode. Only values that fail to *parse*
for a resolved field (`DB_MAXCONNS=abc`) return `InvalidOptionsError`. Error
text names services and fields only — values are never echoed.

Note the deliberate asymmetry with the old file layer: files were strict and
env was best-effort. Now env is the only layer, and it stays best-effort
about keys it does not understand — strict only where a close field shows a
typo is likely.

### Strict mode

`ZEVER_CONFIG_STRICT=1` restores the v0.8 behavior: **every** unknown key
under a wired service prefix fails, with or without a suggestion. The switch
is read from the same environment map `Resolve` and `ApplyEnv` are given, so
it needs no CLI flag:

```sh
ZEVER_CONFIG_STRICT=1 zever serve
```

`config.UnresolvedKeys(env)` lists the keys that were ignored, so a caller
(for example `zever doctor`) can report them; a key with a non-empty
`Suggestion` is a near-miss typo.

## Dotenv files

`.env` (checked in with dummy values) and `.env.local` (git-ignored, real
secrets) share one stdlib-only format:

```sh
# comment lines and blank lines are skipped
export DB_ADAPTER=postgres   # `export` prefix allowed
DB_MAXCONNS=20               # unquoted; ` #` starts a comment
AUTH_JWT_SECRET='a#b ${NO}'  # single quotes: literal, no expansion
DB_DSN="postgres://app:${DB_PASSWORD}@db:5432/app"  # double quotes: escapes + ${VAR}
HOOK='$(not-executed)'       # $(...) and backticks are literal text
```

`${VAR}` expands from the merged map (unset names become empty); expansion
runs bounded passes (max 16 — self-referential values fail). DoS bounds:
10 MiB max file size, 1 MiB max line length. A malformed line fails closed
naming the file and line, never the value.

Library code takes the injectable `Env` map and never calls `os.Setenv`;
only `FromEnv` reads the process environment and the working directory.

## Shared pools (`dedicated_pool`)

Db-backed batteries (`db`, `cache`, `queue`, `search`, `session`,
`idempotency`, `workflow`, `scheduler`, `vectorstore`) share one container
pool per exact DSN by default: point two batteries at the same DSN and the
second borrows the first opener's pool (`owns=false`, never closes it).
`cache`, `session`, `idempotency`, and `scheduler` also accept `dsn` for
their `db`/`postgres` adapters (empty still means a private in-memory
database).

```sh
SEARCH_ADAPTER=db
SEARCH_DSN="postgres://app:secret@db:5432/app?sslmode=require"
SEARCH_DEDICATEDPOOL=false   # default false shares; true opens a private pool
```

`db` is the canonical DB-backed name for `search`, `vectorstore`, and
`workflow` (empty DSN selects the embedded sqlite backend, a postgres URL
selects postgres). The old names (`postgres`, `sqlite`, `pgvector`) stay
registered as aliases; new configuration should use `db`.

- Matching is the exact DSN string after trim (a URL with and without a
  query string are different pools); sqlite shares by cleaned file path and
  `:memory:` never shares.
- The shared pool takes the first opener's pool knobs; a differing nonzero
  `MaxConns` warns without DSN content. Queue, workflow, and scheduler
  under sustained load should use `dedicated_pool: true` with a separate
  database (Solid Queue guidance).
- Env: `SEARCH_DEDICATEDPOOL=true`, `QUEUE_DSN=...`, `SESSION_DSN=...`.

Env names are unaffected by option renames: env matching ignores `_` and
case, so `DB_MAXCONNS` keeps working for `max_conns`.

## Redaction rule

Raw option maps may hold secrets in plain strings. Never log them directly.
`RedactedServices` (built on `Redact`) is the sole display path for logging
or debugging output. Error text names services and fields only — values are
never echoed.

Redaction is key-name based (a denylist of secret-looking key names) plus
type based: option fields tagged `zever:"secret"` in `core/*/options.go`,
or typed `config.Secret`, are always redacted regardless of key name —
the denylist cannot catch oddly named fields such as `access_key_id`.
String values holding URL userinfo under any other key (for example a DSN
under `database_url`) are scrubbed to `scheme://[REDACTED]@host...`;
denylisted keys still redact the whole value.
The type pass covers pointers, slices, maps, nested structs, and
`any`-typed values: a secret tag or type applies to everything beneath it
(a tagged `[]string` or `map[string]string` has every element redacted).
Under URL/DSN-like keys (`*_url`, `*_dsn`, `*_endpoint`, `*_addr`, ...)
scheme-less `user:pass@host` userinfo and `password=...` pairs are scrubbed too.

## Dev-secret warning

`Default()` ships deterministic dev-only secrets where `Validate` requires
them (crypto key). They are public and must never reach production:
supply real secrets via environment.

The crypto dev key is rejected by the local adapter unless explicitly
allowed: set `CRYPTO_ALLOW_DEV_KEY=true` in the env, or run with
`ZEVER_ENV=dev|test|development`. Anything else must supply a
real key (`CRYPTO_KEY`).

`ZEVER_*` names are CLI/test switches only (`ZEVER_INTERACTIVE`,
`ZEVER_NO_HINT`, `ZEVER_JSON`, `ZEVER_ENV`, `ZEVER_DEBUG`,
`ZEVER_CONFIG_STRICT`, test-only `ZEVER_CHROMEDP_NO_SANDBOX`): service
configuration itself is always unprefixed `<SERVICE>_*`.

## Migration from zever.yaml

Service configuration moved from `zever.yaml`/`.yml`/`.json` files to the
environment. Translate each stanza mechanically:

```yaml
# before: zever.yaml
db:
  adapter: postgres
  options:
    dsn: postgres://app:secret@db:5432/app
    max_conns: 20
log:
  adapter: slog
```

```sh
# after: .env
DB_ADAPTER=postgres
DB_DSN=postgres://app:secret@db:5432/app
DB_MAXCONNS=20
LOG_ADAPTER=slog
```

Rules: `adapter:` becomes `<SERVICE>_ADAPTER`; each `options:` key becomes
`<SERVICE>_<KEY>` uppercased (`max_conns` → `DB_MAXCONNS`); one nesting
level flattens with a single `_` (`jwt: {secret: ...}` under auth →
`AUTH_JWT_SECRET`); the `project:` table (if any) is dropped — project
layout is defaults-only for now. Option values that are maps, lists, or
other complex shapes have no string form and cannot move to env: re-check
whether the adapter needs them. Unknown keys that used to fail file
decoding now fail `Resolve` with a did-you-mean hint when they are close to
a known field; the rest are ignored as ambient variables (see
[Ambient-environment safety](#ambient-environment-safety-and-the-typo-rule)).

## Reload: restart-only, except SIGHUP

Configuration is restart-only: editing the env has no effect on a running
process. The generated server (`zever generate server`) watches for SIGHUP
and re-resolves the env, but only three batteries swap atomically via
`container` reloadables — everything else keeps its boot-time value until
the process restarts:

| Battery | SIGHUP behavior |
|---|---|
| `log` (`min_level`) | Swapped: the new level applies to subsequent events, including loggers derived with `With()` before the reload. |
| `ratelimit` (`rate`, `burst`, …) | Swapped: the old limiter is closed only after its in-flight calls drain; each limiter is closed exactly once (the boot-time one by the container at shutdown). |
| `flag` | Swapped when the process holds the client through `container.ReloadableFlag`. The `static` adapter additionally re-reads its file per evaluation when `reload: true` (last-good on failure), with no signal needed. |

A reload is all-or-nothing across batteries: the new log and rate-limit
state are opened first and swapped in together; if either fails, the old
state stays fully in place and a single warning is logged.

Deliberately NOT reloaded (restart required): service adapters, DSNs and
pool knobs, credentials and secrets, TLS material, listen addresses, and
every other battery. If reloading anything else looks tempting, restart
instead — a half-reloaded process (new credentials against old pools) is
worse than a brief restart.
