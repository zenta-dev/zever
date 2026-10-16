# Threat model: password

Audited against `core/password/` (`password.go`, `options.go`) and
`adapters/password/argon2/argon2.go` at WS6 time.

## Assets

- Candidate passwords (in memory, transient) and stored PHC hashes
  (`$argon2id$v=19$m=…,t=…,p=…$salt$hash`).

## Trust boundaries

- The password crosses caller → hasher once per Hash/Verify; the stored hash
  crosses the database. Hashes are **not** secrets in the same sense as
  passwords, but they are offline-attack material: whoever steals the table
  can guess weak passwords at leisure.

## What zever guarantees

- Argon2id only (adapter string `argon2id`, package `argon2`), with
  opinionated defaults (`Time=3`, `Memory=64 MiB`, `Threads=4`,
  `SaltLen=16`, `KeyLen=32`) and enforced ranges (time 1–10, memory
  8 MiB–1 GiB, threads 1–16, salt 8–64 B, key 16–128 B). Out-of-range
  options fail with `ErrInvalidHash`.
- Fresh CSPRNG salt per hash (`crypto/rand` via `randRead`); overlong
  inputs rejected with `ErrPasswordTooLong` before hashing; comparison is
  constant-time (`subtle.ConstantTimeCompare`).
- Stored-hash parsing is defensive: malformed PHC strings fail instead of
  hashing, and crafted parameters cannot force unbounded memory (range
  checks in `decodeHash` bound `m/t/p` before `IDKey` runs).
- `NeedsRehash` reports drift from current parameters so logins can upgrade
  hashes opportunistically. The two `uint32(len(...))` G115 `nolint`s are
  sound: lengths come from a successfully decoded stored hash, already
  length-bounded by the checks above.

## What the caller must do

- Call `NeedsRehash` on successful login and re-`Hash` when true; otherwise
  cost upgrades never reach existing users.
- Rate-limit and anomaly-gate login attempts; argon2id is deliberately
  expensive (CPU+RAM), so unauthenticated verify endpoints are themselves a
  DoS surface — throttle before hashing, not after.
- Never log passwords or hashes; keep the users table (hashes + salts are
  inside) under least-privilege access; require strong password policy at
  enrollment (the hasher cannot fix `password123`).

## Explicit non-guarantees

- No pepper, no server-side secret input: anyone with DB read access gets the
  exact verifier input. Peppering is caller-owned (e.g. HMAC the password
  with a KMS key before `Hash`, applying the same step in `Verify`).
- No breach-detection, lockout, or MFA: authentication policy above hashing
  is the app's job.
