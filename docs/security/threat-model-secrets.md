# Threat model: secrets

Audited against `core/secrets/` (`secrets.go`, `options.go`) and
`adapters/secrets/{env,vault}` at WS6 time.

## Assets

- Every secret value the app reads at runtime. The secrets backend is the
  highest-value target in the deployment: compromise reads everything.

## Trust boundaries

- Secret *names* cross caller → backend and are interpolated into storage
  paths/URLs by some adapters — names are protocol, not free text.
- The vault token crosses caller → Vault in the `X-Vault-Token` header;
  the token file crosses disk → process at startup.

## What zever guarantees

- `ValidateName` rejects traversal and injection: names must be non-empty,
  match `[a-zA-Z0-9._-]+`, and must not contain `/` or `..`
  (`core/secrets/secrets.go`).
- The vault adapter requires an `https` Addr (the token travels in cleartext
  otherwise); plain http needs explicit `allow_insecure: true`, which logs a
  warning at open. Round-trips use a client with a TLS 1.2 floor.
- A vault `token_file` readable beyond its owner (broader than `0600`)
  fails closed with `ErrInsecureTokenFilePerms` at validation and at token
  resolution; opting out (`allow_insecure_token_file_perms: true`) logs a
  warning when the file is actually permissive (CHANGELOG v0.7.0 P2-11b).
- The KV mount path is escaped per slash-separated segment before URL
  interpolation; empty and dot segments are rejected, so reserved characters
  (`?`, `#`, `%`, …) cannot shift path segments (CHANGELOG v0.7.0 P2-11c).
- The `env` adapter scopes to the `ZEVER` prefix by default
  (`config/default.go`).

## What the caller must do

- Keep secret names static, alphanumeric, and caller-chosen — never build
  them from user input even though `ValidateName` exists (defense in depth).
- Restrict token files to `0600`, serve Vault over https with a trusted CA,
  and scope Vault tokens/policies to the exact mount paths the app needs.
- Treat `allow_insecure*` flags as test-only tripwires: their warnings in
  logs are findings. Never enable them in any environment holding real
  secrets.
- Remember the `env` adapter has no audit trail or versioning: prefer Vault
  (or equivalent) for production, with rotation runbooks.

## Explicit non-guarantees

- The secrets battery does not rotate, version, or lease secrets by itself;
  TTLs and renewal against Vault are caller-owned.
- Process memory is not scrubbed: a core dump or `/proc` reader sees live
  secret bytes regardless of file permissions.
