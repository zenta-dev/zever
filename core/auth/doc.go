// Package auth provides authentication with swappable adapters.
//
// It owns token Issue, Verify and Revoke plus typed claims cloning. It does
// not hash passwords or encrypt secrets; password and crypto are opt-in
// companions.
//
// Type safety: Auth plus typed Options plus Factory.
// JWTOptions, SessionOptions and OIDCOptions are per-adapter structs;
// MinSecretLen and DefaultTimeout bound adapter inputs. Unsupported features
// fail closed with ErrNotSupported.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with
// zero-infra defaults for tests. Configuration comes from the environment
// only: .env, .env.local, and the process environment, with keys named
// <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Auth(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. Auth closes with Close() error and no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed auth:. Token bytes
// are secrets and never echoed; only service and field names appear in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// Token Value, JWT Secret and OIDC client material never appear in errors or
// logs; JWT secrets must meet MinSecretLen bytes.
//
// Performance: DefaultTimeout bounds adapter operations; claims Clone copies
// only known map and slice shapes. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package auth
