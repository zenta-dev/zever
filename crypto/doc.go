// Package crypto encrypts and authenticates bytes with swappable adapters.
//
// It seals plaintext at rest through Encrypt and Decrypt plus HMAC and signatures. It is not a key manager, transport layer, or password hasher; load keys from secrets.
//
// Type safety: Crypto embeds Encryptor and Signer plus typed Options plus Adapter enum plus Factory. Key and SignKey are base64 32-byte and 64-byte secrets validated before factory lookup. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env CRYPTO_<FIELD> (no prefix, e.g. CRYPTO_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Crypto(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Crypto holds no handle and defines no Close; key material lives in memory only.
//
// Errors: sentinel errors, errors.Is compatible, prefixed crypto:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Keys are base64 secrets checked for exact size and never logged; load the key from secrets, never hard-code it.
//
// Performance: in-memory AES-GCM with no pooling and no network calls. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package crypto
