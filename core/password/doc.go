// Package password password-hashing facade with swappable adapters.
//
// It hashes, verifies, and rehash-checks passwords through Hasher with typed
// Options. It is not a user store and it does not issue tokens; auth calls it
// on the verification path before issuing anything.
//
// Type safety: Hasher plus typed Options plus Adapter enum plus Factory. Options
// carry Time, Memory, Threads, SaltLen, and KeyLen with validated ranges and
// Argon2id defaults. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env PASSWORD_<FIELD> (no prefix, e.g. PASSWORD_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Password(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Hasher holds no resources and exposes no Close or Shutdown; hashing is pure CPU work.
//
// Errors: sentinel errors, errors.Is compatible, prefixed password:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Salts come from crypto/rand, verification is constant-time, passwords over 1024 bytes are rejected, and stored-hash validation returns the bare ErrInvalidHash sentinel.
//
// Performance: defaults are Time 3 iterations, Memory 65536 KiB (64 MiB), Threads 4, SaltLen 16 bytes, KeyLen 32 bytes. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package password
