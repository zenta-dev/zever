# config

Layered service configuration: zero-infrastructure defaults, overlaid by a
config file, overlaid by environment variables. Later layers win:
`defaults < file < env`.

## Precedence

1. **Default** — `Default()` returns zero-infrastructure adapters (memory,
   local, sqlite, log, noop, ...; a few batteries default to the `db`
   adapter instead, e.g. `outbox`, `search`, `vectorstore`) so tests run
   without manual setup. The
   only dev secret is the crypto key, which `Validate` requires; it is
   deterministic and must never ship to production.
2. **File** — `Load(path)` decodes YAML or JSON strictly. Unknown services
   and unknown fields are errors; typos fail fast.
3. **Env** — variables carry **no prefix**: `DB_ADAPTER=postgres`,
   `DB_MAXCONNS=20`, `AUTH_JWT_SECRET=...`. Unknown variables are ignored.
   Exception: the `secrets` env adapter requires a non-empty `prefix`
   (default `ZEVER`, so `ZEVER_FOO` maps to secret `FOO`), and CLI/TUI
   switches use `ZEVER_INTERACTIVE` / `ZEVER_NO_HINT`. Test-only
   `ZEVER_CHROMEDP_NO_SANDBOX` follows the same prefixed convention.

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
file/env; mailer defaults to `log` with a dummy localhost SMTP host/port
because its `Validate` requires them; secrets defaults to `env` with a
`ZEVER` prefix because its `Validate` requires a non-empty prefix).

## File example (YAML)

```yaml
db:
  adapter: postgres
  options:
    dsn: postgres://app:secret@db:5432/app
    max_conns: 20
log:
  adapter: slog
```

## Shared pools (`dedicated_pool`)

Db-backed batteries (`db`, `cache`, `queue`, `search`, `session`,
`idempotency`, `workflow`, `scheduler`, `vectorstore`) share one container
pool per exact DSN by default: point two batteries at the same DSN and the
second borrows the first opener's pool (`owns=false`, never closes it).
`cache`, `session`, `idempotency`, and `scheduler` also accept `dsn` for
their `db`/`postgres` adapters (empty still means a private in-memory
database).

```yaml
search:
  adapter: db
  options:
    dsn: "postgres://app:secret@db:5432/app?sslmode=require"
    dedicated_pool: false   # default false shares; true opens a private pool
```

`db` is the canonical DB-backed name for `search`, `vectorstore`, and
`workflow` (empty DSN selects the embedded sqlite backend, a postgres URL
selects postgres). The old names (`postgres`, `sqlite`, `pgvector`) stay
registered as aliases, so existing files keep resolving; new files should
use `db`.

- Matching is the exact DSN string after trim (a URL with and without a
  query string are different pools); sqlite shares by cleaned file path and
  `:memory:` never shares.
- The shared pool takes the first opener's pool knobs; a differing nonzero
  `MaxConns` warns without DSN content. Queue, workflow, and scheduler
  under sustained load should use `dedicated_pool: true` with a separate
  database (Solid Queue guidance).
- Env: `SEARCH_DEDICATEDPOOL=true`, `QUEUE_DSN=...`, `SESSION_DSN=...`.
  Downgrade note: old binaries reject the new fields under strict
  decoding, so remove `dedicated_pool`/`dsn` lines before downgrading.

Only `.yaml`, `.yml`, and `.json` decode; anything else fails with
`ErrUnsupportedFormat`. Null or empty service blocks decode to zero values.

File keys are snake_case and strict: the pre-rename flat spellings are
rejected as unknown fields. Migration:

| Old key | New key |
|---|---|
| `apikey` | `api_key` |
| `baseurl` | `base_url` |
| `maxconns` / `minconns` | `max_conns` / `min_conns` |
| `maxconnlifetime` / `maxconnidletime` | `max_conn_lifetime` / `max_conn_idle_time` |
| `maxttl` | `max_ttl` |
| `clientid` | `client_id` |
| `visibilitytimeout` / `polltimeout` | `visibility_timeout` / `poll_timeout` |
| `minlevel` | `min_level` |
| `sweepinterval` / `maxentries` | `sweep_interval` / `max_entries` |
| `poolsize` / `minidleconns` / `pooltimeout` / `connmaxidletime` / `connmaxlifetime` | `pool_size` / `min_idle_conns` / `pool_timeout` / `max_conn_idle_time` / `max_conn_lifetime` |
| `secretkey` / `webhooksecret` | `secret_key` / `webhook_secret` |
| `autoapprove` / `maxwebhookbytes` | `auto_approve` / `max_webhook_bytes` |
| `anonymousid` / `grouptype` | `anonymous_id` / `group_type` |
| `maxpropertiesbytes` / `maxproperties` | `max_properties_bytes` / `max_properties` |
| `tmpdir` / `maxoutputbytes` | `tmp_dir` / `max_output_bytes` |
| `latexcommand` / `latexpdftoppm` / `latexruns` | `latex_command` / `latex_pdf_to_ppm` / `latex_runs` |
| `latexsandbox` / `latexallowunsafesource` | `latex_sandbox` / `latex_allow_unsafe_source` |
| `buffersize` / `maxhandlers` | `buffer_size` / `max_handlers` |
| `handlertimeout` / `closetimeout` | `handler_timeout` / `close_timeout` |
| `projectid` / `serviceaccount` | `project_id` / `service_account` |
| `maxinflight` / `allowinsecure` | `max_in_flight` / `allow_insecure` |
| `maxmessagesize` | `max_message_size` |
| `maxdownloadbytes` / `maxpixels` | `max_download_bytes` / `max_pixels` |
| `derivedttl` / `presignttl` / `maxduration` | `derived_ttl` / `presign_ttl` / `max_duration` |
| `accesskeyid` / `secretaccesskey` | `access_key_id` / `secret_access_key` |
| `accountsid` / `authtoken` / `fromnumber` | `account_sid` / `auth_token` / `from_number` |
| `servicename` / `setglobals` | `service_name` / `set_globals` |
| `cafile` / `certfile` / `keyfile` | `ca_file` / `cert_file` / `key_file` |
| `sampleratio` / `attrvaluelimit` | `sample_ratio` / `attr_value_limit` |
| `modelpath` / `policypath` | `model_path` / `policy_path` |
| `idlettl` | `idle_ttl` |
| `maxretries` / `replaytolerance` | `max_retries` / `replay_tolerance` |
| `allowprivatetargets` / `queueadapter` / `queueopts` / `deadlettertopic` | `allow_private_targets` / `queue_adapter` / `queue_opts` / `dead_letter_topic` |
| `subdomainregex` | `subdomain_regex` |
| `publicurl` / `policysync` / `syncfail` | `public_url` / `policy_sync` / `sync_fail` |
| `accountid` / `urlbase` | `account_id` / `url_base` |

Env names are unaffected: env matching ignores `_` and case, so
`DB_MAXCONNS` keeps working.

## Env scheme

`<SERVICE>_<FIELD>=value`, uppercased, no prefix (except `secrets`
prefix and `ZEVER_*` CLI switches noted above):

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

### Best-effort rule and hostile-environment safety

Env matching never errors on unknown input: the head (up to the first `_`)
must name a known service, the remainder must be non-empty, and the field
must resolve — otherwise the variable is silently ignored. Strict mode
would fail here: bare names like `PATH`, `HOME`, and `USER` contain no `_`
at all, and plausible heads like `AUTH` in `AUTH_TOKEN` collide with real
service names. Only values that fail to *parse* for a resolved field
(`DB_MAXCONNS=abc`) return `InvalidOptionsError`.

## Strictness asymmetry

File and env behave differently on purpose: files are strict (unknown
service or field fails the load — typos fail fast), while env is
best-effort (unknown variables ignored — stray process environment cannot
break startup). Both paths stay type-safe through the generic strict
decoder (file) and a kind-switched reflection setter (env).

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

## Dev-secret warning

`Default()` ships deterministic dev-only secrets where `Validate` requires
them (crypto key). They are public and must never reach production:
supply real secrets via file or environment.

The crypto dev key is rejected by the local adapter unless explicitly
allowed: set `crypto.options.allow_dev_key: true` in the config file, or
run with `ZEVER_ENV=dev|test|development`. Anything else must supply a
real key (`crypto.options.key`).

## Discovery

`Load("")` probes `zever.yaml`, `zever.yml`, then `zever.json` in the
working directory; first hit wins, absent files skip silently. An explicit
path must exist and decode — any failure returns `nil` plus the error, as
does any merge, env, or validation failure.

## Reload: restart-only, except SIGHUP

Configuration is restart-only: editing the file (or the environment) has no
effect on a running process. The generated server (`zever generate server`)
watches for SIGHUP and re-reads the config file, but only three batteries
swap atomically via `container` reloadables — everything else keeps its
boot-time value until the process restarts:

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

## Editor autocomplete and validation

`config/schema/` has a JSON Schema for `zever.yaml`/`.yml`/`.json` (plus a
TOML-format copy for TOML-aware tooling) — see `config/schema/README.md`
for how to wire it into your editor.
