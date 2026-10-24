// Package crypto encrypts and authenticates bytes with swappable adapters.
//
// It seals plaintext at rest through Encrypt and Decrypt plus HMAC and signatures. It is not a key manager, transport layer, or password hasher; load keys from secrets.
//
// Type safety: Crypto embeds Encryptor and Signer plus typed Options plus Factory. Key and SignKey are base64 32-byte and 64-byte secrets validated before the factory builds the backend. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
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
// Example: see Example in example_test.go.
package crypto
