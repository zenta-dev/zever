# Threat model: config

Audited against `config/` (`default.go`, `dotenv.go`, `env.go`, `redact.go`,
`validate.go`, `validate_adapters.go`) at WS6 time. Claims cite code, not intent.

## Assets

- Every secret in the process: crypto keys, JWT secrets, DB DSNs, API tokens,
  vault tokens. `config.Default()` ships one deterministic public secret, the
  dev crypto key (`core/crypto/devkey.go`, `DevCryptoKey` = base64 of
  `0123456789...`).
- The resolved `Config`: which adapters run and with what options.

## Trust boundaries

- Dotenv files (`.env`, `.env.local`) and the process environment are
  **untrusted input** at load time. `FromEnv()` parses the files fail-closed:
  a malformed line is an error naming the file and line, never the value
  (`dotenv.go`). There is no service config file: a legacy
  `zever.yaml`/`.yml`/`.json` in the cwd is never read, and nothing warns
  about it either, so it is not a config input path.
- Env overlay is **best-effort** for unknown variables: unknown heads and
  bare names are silently ignored so a hostile/noisy environment cannot
  break startup; only keys that resolve under a wired service prefix and
  values that fail to parse for a *resolved* field error out (`env.go`,
  `CLAUDE.md`).
- Display/logging is a boundary: option maps may hold plaintext secrets.

## What zever guarantees

- `Default()` is zero-infrastructure (memory, local, sqlite, log, noop) so
  tests run with no external services; the only deterministic secret is the
  crypto dev key, documented "never production" (`default.go`).
- The local crypto adapter **rejects the dev key by default**: accepted only
  with `CRYPTO_ALLOW_DEV_KEY=true` (the `allow_dev_key` option) or `ZEVER_ENV`
  in {dev, test, development} (trimmed, case-insensitive `IsDevEnv`); every
  other environment fails closed with `DevKeyError` (`core/crypto/devkey.go`,
  `STABILITY.md`).
- Secrets are redacted by type (`config.Secret`) and by the `zever:"secret"`
  struct tag, plus a key-name denylist as defense-in-depth; DSN/URL-shaped
  values are scrubbed of userinfo and `password=` pairs (`redact.go`).
  A guard test fails when a secret-looking field in `core/*/options.go` is
  neither tagged nor allowlisted (CHANGELOG v0.7.0).
- `ValidateAdapters` fails fast on unknown adapters with did-you-mean hints;
  `ValidateEnabledAdapters` scopes the check to configured services so
  unconfigured batteries cannot break boot.

## What the caller must do

- Never ship `Default()` secrets: supply real keys via env in any
  non-dev environment.
- Never log raw option maps; render through `RedactedServices`/`Redact`.
- Set `ZEVER_ENV` explicitly (unset counts as non-dev and fails closed).
- Treat `.env.local` as secret-bearing: restrict file permissions and keep
  it out of version control when it holds real credentials.

## Explicit non-guarantees

- Env-var names are not authenticated: anyone who can write the process
  environment can change resolved config. That is by design (12-factor), not
  a vulnerability, but it means the environment is trusted.
- Redaction is display-only; it does not encrypt secrets at rest or in memory.
