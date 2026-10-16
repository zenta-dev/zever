# ADR: Asymmetric JWT support (EdDSA + RS256)

- Status: **proposed** (not accepted, not implemented — no code changes ship
  with this ADR).
- Date: 2026-10-16.
- Context: `adapters/auth/jwt` is HS256-only by design; the keyfunc rejects
  any token whose `alg` is not exactly `HS256` (plus `jwt.WithValidMethods`),
  blocking `none`/RS256-confusion attacks. Docs state "asymmetric via custom
  adapter". Deployments that must verify third-party or multi-service tokens
  (federation, HSM-backed signers, cross-org audiences) need EdDSA and RS256
  without giving every verifier a shared HMAC secret.
- Related: `../security/threat-model-auth.md`; kid rotation semantics in
  `adapters/auth/jwt/jwt.go`; `STABILITY.md` kid-less timeline (no removal
  date; bounded by `jwt.max_ttl`).

## Proposal

1. **Algorithm pinning per key, never per token.** Each configured key
   declares its algorithm (`HS256`, `Ed25519`, `RS256`); verification selects
   the key by `kid`, then requires the token's `alg` to equal that key's
   declared algorithm exactly. Unknown kids, missing kids (once asymmetric
   keys exist — see migration), and `alg` mismatches fail closed. `none` is
   never valid for any key. This preserves today's confusion resistance while
   adding two algorithms.
2. **Key configuration.** Extend `auth.JWTKey` (additive fields only, so the
   `auth.JWTOptions` comparability break stays the one already documented):
   `alg` (required for new entries, defaults to `HS256` for existing ones),
   plus key material by algorithm: HMAC keeps `secret`; EdDSA takes a
   base64 seed/private key plus public key (or JWK); RS256 takes public key
   for verify-only deployments and private key where this service mints.
   Private material gets the `zever:"secret"` tag so redaction covers it.
3. **Key rotation guarantees carried over.** Active-key signing via
   `ActiveKID` (empty = first entry); retired keys stay listed until their
   tokens expire; `exp` required, `nbf` enforced when present, same `leeway`
   semantics; issuer/audience enforcement unchanged; revocation store
   semantics unchanged (per-process default warns; shared store for
   multi-instance).
4. **RS256 hardening.** Minimum 2048-bit moduli (reject smaller at
   `Validate`); require `e = 65537` unless an explicit allowlist entry says
   otherwise; parse public keys with strict PKIX/SPKI handling and reject
   multi-prime and private material in verify-only config (fail closed on
   accidental private-key exposure in verifiers).
5. **EdDSA hardening.** Ed25519 only (no Ed448); reject malleable/small-order
   public keys at load; signatures must be exactly 64 bytes; one canonical
   encoding accepted.
6. **JWKS (explicit non-goal for v1).** No automatic remote key fetching in
   the JWT adapter: keys are local config. OIDC keeps covering
   discovery/JWKS-backed verification. If JWKS is ever added, it needs its
   own ADR (fetch bounding, caching, rotation races — cf. fix/s6-oidc #610).

## Test plan

- Unit: per-algorithm issue/verify round-trips; `alg`-mismatch matrix
  (token signed EdDSA presented against an HS256 `kid` and every other
  permutation — all reject); `none` against every key type; unknown-kid and
  missing-kid cases; weak-RSA (1024-bit, e=3) rejected at `Validate`;
  small-order Ed25519 key rejected at load; private key in verify-only
  config rejected.
- Rotation: mixed HMAC+EdDSA key sets; retire-signing-key-keep-verify until
  expiry; legacy `Secret` fallback behavior unchanged for kid-less tokens.
- Conformance: extend the `auth` conformance kit (`core/auth/authtest`) so
  every adapter runs the new vectors; fuzz the keyfunc dispatch
  (`alg`×`kid` matrix, 10–20s burst like existing adapter fuzz targets).
- Interop: verify reference vectors against `golang-jwt/jwt` and one
  external implementation (e.g. a Python `PyJWT` script in CI, vectors
  committed under `testdata/`).

## Migration

1. Ship behind config only: no operator action changes existing behavior;
   HS256-only deployments see zero wire change.
2. Operators add the asymmetric key to `Keys`, set `ActiveKID` to it once
   ready, and keep the HMAC key listed until `max_ttl` bounds out old tokens.
3. Kid-less acceptance: with asymmetric keys configured, kid-less tokens
   verify against legacy `Secret` only when it is set (unchanged rule); to
   retire kid-less acceptance, remove `Secret` after expiry (same procedure
   as `STABILITY.md` documents today).
4. Docs: update the JWT guide ("HS256 only" becomes a per-key statement),
   `STABILITY.md` if any default changes, and `CHANGELOG.md` with a
   `**Breaking:**` entry if `JWTKey` gains required fields.

## Risks

- Larger parsing surface (RSA key parsing, JWK handling) in the auth hot
  path — mitigate with load-time validation and the test plan above.
- Operator error (weak RSA, private keys in verifiers) — mitigate with
  fail-closed `Validate` rules, never warnings.
