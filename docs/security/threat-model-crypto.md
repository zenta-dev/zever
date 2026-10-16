# Threat model: crypto

Audited against `core/crypto/` (`crypto.go`, `options.go`, `devkey.go`) and
`adapters/crypto/{local,kms}` at WS6 time.

## Assets

- Plaintext the caller encrypts, the 32-byte data key, the 64-byte signing
  key, and KMS credentials/endpoint configuration.

## Trust boundaries

- Ciphertext and signatures cross untrusted storage/transport by design;
  security rests entirely on key secrecy and algorithm choice inside the
  adapters.
- The KMS endpoint and `X`-style credential headers cross the network to
  infrastructure the operator selects.

## What zever guarantees

- `Options.Validate` checks key *shape* before factory lookup (base64,
  32 bytes for `Key`, 64 bytes for `SignKey` when present; envelope mode
  requires `KeyID`; endpoint needs scheme+host), so malformed material fails
  fast and closed.
- The deterministic dev key (`DevCryptoKey`, public by design) is rejected
  unless `allow_dev_key: true` or `ZEVER_ENV` ∈ {dev, test, development}
  (trimmed, case-insensitive). Unset `ZEVER_ENV` counts as non-dev.
- KMS failures fail closed: base64 decode errors surface as `ErrInvalidKey`
  instead of deriving keys from empty material; missing/malformed private
  keys fail `Sign`/`Verify` with `ErrKeyNotFound` instead of panicking
  (CHANGELOG v0.7.0 P2-11).
- `Open` validates before factory lookup, so no adapter ever sees invalid
  options.

## What the caller must do

- Supply real keys from secrets management in every non-dev environment;
  never check keys into the repo or bake them into images.
- Plan rotation as **re-encryption**: the local adapter embeds no
  key-id/version in its output, so rotating `Key` permanently orphans
  ciphertext sealed under the old key (`options.go`). Dual-read or
  versioned-envelope schemes are caller-owned until a versioned adapter
  exists.
- Prefer envelope/KMS modes for production data; pin the KMS endpoint to
  https with a trusted CA; scope KMS identities to the exact key IDs used.
- Zero plaintext buffers when the app is done with them; the adapter cannot
  clear the caller's copies.

## Explicit non-guarantees

- No algorithm agility or key versioning in the local adapter output format.
  No side-channel promises beyond what Go's stdlib primitives provide.
- `AllowDevKey`/`ZEVER_ENV` is an environment gate, not an authentication:
  anyone controlling the environment controls the gate.
