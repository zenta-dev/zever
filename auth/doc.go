// Package auth provides authentication with swappable adapters.
//
// It owns token Issue, Verify and Revoke plus typed claims cloning. It does
// not hash passwords or encrypt secrets; password and crypto are opt-in
// companions.
//
// Type safety: Auth plus typed Options plus Adapter enum plus Factory.
// JWTOptions, SessionOptions and OIDCOptions are per-adapter structs;
// MinSecretLen and DefaultTimeout bound adapter inputs. Unsupported features
// fail closed with ErrNotSupported.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests. Config file plus env AUTH_<FIELD> (no prefix,
// e.g. AUTH_ADAPTER). See config/README.md.
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
// Example: see ExampleOpen in example_test.go.
package auth
