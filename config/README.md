# config

Layered service configuration: zero-infrastructure defaults, overlaid by a
config file, overlaid by environment variables. Later layers win:
`defaults < file < env`.

## Precedence

1. **Default** — `Default()` returns zero-infrastructure adapters (memory,
   local, sqlite, log, noop, ...) so tests run without manual setup. The
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
| ai            | anthropic, openai, gemini           |
| analytics     | log, posthog                        |
| auth          | jwt, session, oidc                  |
| billing       | stub, stripe, paddle                |
| cache         | memory, redis                       |
| crypto        | local                               |
| db            | sqlite, postgres                    |
| document      | local, remote, latex                |
| eventbus      | memory, redis                       |
| flag          | static, firebase                    |
| geo           | google, static, osm                 |
| i18n          | embed, remote                       |
| idempotency   | memory, redis                       |
| lock          | memory, redis                       |
| log           | noop, zerolog, slog                 |
| mailer        | log, smtp                           |
| media         | local, s3                           |
| notification  | log, twilio, fcm                    |
| observability | noop, stdout, otlp                  |
| password      | argon2id                            |
| payment       | stub, stripe, paddle                |
| permission    | noop, rbac, casbin                  |
| queue         | memory, redis                       |
| ratelimit     | memory, redis                       |
| router        | fiber, stdhttp                      |
| scheduler     | embedded                            |
| search        | postgres, meilisearch, sqlite       |
| secrets       | env                                  |
| session       | memory, redis                       |
| storage       | local, s3, r2                       |
| tenant        | single, header                      |
| vectorstore   | sqlite, pgvector, qdrant            |
| webhook       | http, queue, sqlite                 |
| workflow      | memory                              |

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
| `poolsize` / `minidleconns` / `pooltimeout` / `connmaxidletime` / `connmaxlifetime` | `pool_size` / `min_idle_conns` / `pool_timeout` / `conn_max_idle_time` / `conn_max_lifetime` |
| `conn_max_idle_time` / `conn_max_lifetime` | `max_conn_idle_time` / `max_conn_lifetime` |
| `secretkey` / `webhooksecret` | `secret_key` / `webhook_secret` |
| `autoapprove` / `maxwebhookbytes` | `auto_approve` / `max_webhook_bytes` |
| `anonymousid` / `grouptype` | `anonymous_id` / `group_type` |
| `maxpropertiesbytes` / `maxproperties` | `max_properties_bytes` / `max_properties` |
| `tmpdir` / `maxoutputbytes` | `tmp_dir` / `max_output_bytes` |
| `latexcommand` / `latexpdftoppm` / `latexruns` | `latex_command` / `latex_pdf_to_ppm` / `latex_runs` |
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

## Dev-secret warning

`Default()` ships deterministic dev-only secrets where `Validate` requires
them (crypto key). They are public and must never reach production:
supply real secrets via file or environment.

## Discovery

`Load("")` probes `zever.yaml`, `zever.yml`, then `zever.json` in the
working directory; first hit wins, absent files skip silently. An explicit
path must exist and decode — any failure returns `nil` plus the error, as
does any merge, env, or validation failure.
