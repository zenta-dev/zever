# Threat model: session

Audited against `core/session/` (`session.go`, `id.go`, `rotate.go`,
`options.go`), `adapters/session/{memory,redis,db,cookie}` at WS6 time.
Note: redis/db key-hash hardening (PR #618) and `SameSite=None`→`Secure`
(PR #619) are **open proposals, not merged** — the guarantees below describe
merged code only.

## Assets

- Session IDs: bearer tokens. Whoever presents one impersonates the user.
- Session data (server-side) and the session cookie (client-side).

## Trust boundaries

- The cookie crosses the network on every request; the store backend
  (memory/redis/db) may cross the network to shared infrastructure.
- Expired sessions are indistinguishable from missing ones (`ErrNotFound`);
  absence carries no information about whether an ID ever existed.

## What zever guarantees

- IDs are 256-bit bearer secrets: 32 bytes from `crypto/rand`, hex-encoded
  to 64 lowercase chars (`id.go`). UUIDv7 is deliberately not used (its
  timestamp prefix leaks creation time). IDs must never be logged or echoed:
  `ValidateID`/`InvalidIDError` carry only the length.
- `Rotate` mitigates fixation: after privilege change (notably login), call
  it and switch the client to the fresh ID; only data travels, never the ID
  or timestamps; orphaned old sessions die by expiry.
- Cookie defaults are secure: `Secure` and `HttpOnly` default on,
  `SameSite=Lax` by default, session-cookie lifetime by default; `__Host-`
  forces `Secure` + `Path=/` + no `Domain`, `__Secure-` forces `Secure`, with
  prefix constraints winning over conflicting fields (`cookie.go`).
  The `http.Cookie` literal's G124 `nolint` is sound because all three
  attributes are set/defaulted above the literal.
- The sessions guide documents CSRF defense (same-origin `Origin`/`Referer`
  checks, no state on `GET`, synchronizer tokens, header tokens for APIs) as
  docs-only guidance; `SameSite=Lax` alone is explicitly not enough.

## What the caller must do

- Call `Rotate` on every privilege escalation (login, role grant, password
  reset). Set absolute expiries; do not mint immortal sessions.
- Prefer the `__Host-` prefix; never turn off `Secure`/`HttpOnly` except for
  local plaintext dev. Verify `Origin`/`Referer` on state-changing requests
  to cookie sessions; use `Authorization`-header tokens (auth battery) for
  programmatic APIs.
- Until store key-hashing merges, assume store keys are reversible to bearer
  IDs: restrict store (Redis/DB) read access as strictly as the session
  cookie itself, and prefer short TTLs.

## Explicit non-guarantees

- The session layer provides no CSRF middleware (deliberately docs-only) and
  no absolute-session cap: concurrent-session limits and idle timeouts are
  caller-owned.
- Server-side data is not encrypted by the store: anyone with store read
  access sees session contents. Encrypt sensitive fields at the app layer or
  keep them out of sessions.
