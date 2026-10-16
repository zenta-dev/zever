# Threat model: auth

Audited against `core/auth/` (`auth.go`, `options.go`, `errors.go`,
`revocation/`) and `adapters/auth/{jwt,session,oidc}` at WS6 time.
Asymmetric algorithms (EdDSA, RS256) are **not implemented**; see `../adrs/2026-10-16-asymmetric-jwt.md` for the proposal.

## Assets

- Signing secrets (HMAC keys), minted tokens (bearer credentials), verified
  `Claims` (identity the app trusts), the revocation set.

## Trust boundaries

- Tokens cross the network per request and are attacker-presentable: anyone
  holding a valid token is the subject until expiry or revocation.
- `Token.Value` and raw OIDC tokens are secrets: never logged or echoed in
  errors (adapters wrap with `%w` sentinels, never the value).

## What zever guarantees

- **Algorithm pinning (JWT):** HS256-only. The keyfunc rejects any token
  whose `alg` header is not exactly `HS256`, additionally constrained with
  `jwt.WithValidMethods` — blocking `none`/RS256-confusion attacks
  (`adapters/auth/jwt/jwt.go`). Custom claims colliding with registered
  names (`iss sub aud exp nbf iat jti`) are rejected at Issue time.
- **Key strength floor:** rotation-key secrets under 32 bytes fail
  `Validate` (`MinSecretLen`); the legacy single-secret length is
  adapter-enforced.
- **Rotation without outage:** `Keys[{kid,secret}]` + `ActiveKID` (empty
  means `Keys[0]`); new tokens carry the active `kid`, verification selects
  by `kid`, kid-less legacy tokens verify against `Secret` while it is set,
  unknown kids fail closed. Retired keys must stay listed until their tokens
  expire (bounded by `max_ttl` when set).
- **Time enforcement:** `exp` required, `nbf` enforced when present, both
  honoring `leeway` (default 0 = strict; ~30s typical for skewed fleets).
  Sub-second TTLs round down (second-precision wire); keep TTLs ≥ 1s.
- **Tenant separation:** configured issuer/audience are enforced, and when
  empty, tokens *carrying* issuer/audience are rejected — so one shared HMAC
  secret cannot mint cross-tenant tokens.
- **Revocation honesty:** the default memory store is per-process (lost on
  restart, invisible to siblings); `New` logs a one-time `slog` WARN and
  exposes it via `WarningReporter`. Multi-instance deployments must inject a
  shared store (`revocation/redis` or custom). Key material is zeroed on
  `Close` (pinned by `hardening_test.go`).
- **OIDC:** discovery and verification run through a safe client (TLS 1.2
  floor, private-address dial guard, no redirects); `AllowInsecure` opts a
  test issuer into http/private addresses only. `Issue`/`Revoke` correctly
  fail with `ErrNotSupported` (the IdP owns the lifecycle); empty tokens are
  rejected before any network use.
- Session-backed auth inherits the session battery's guarantees; its `Store`
  is required (nil fails `New`).

## What the caller must do

- Generate every secret from a CSPRNG at ≥ 32 bytes; store in secrets
  management; rotate via `Keys`+`ActiveKID`, then remove the legacy
  `Secret` only after all kid-less tokens expired. Set `max_ttl` to bound
  rotation windows and `leeway` only as large as clock skew demands.
- Inject a shared revocation store for anything multi-instance; treat the
  memory-store WARN as a finding, not noise. Enforce short TTLs so stolen
  tokens die quickly.
- For OIDC, pin the expected issuer/client ID, keep `AllowInsecure` false in
  any non-test environment, and map only `email_verified`-true addresses to
  trusted identity where the IdP may leave verification to the user.

## Explicit non-guarantees

- No asymmetric verification today: RS256/EdDSA tokens are rejected, and any
  deployment needing them must supply a custom `auth.Auth` adapter (or wait
  for the ADR to be accepted and implemented).
- Revocation is opt-in infrastructure: without a shared store, `Revoke` only
  constrains the issuing process.
