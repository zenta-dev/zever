// Package permission authorization facade with swappable adapters.
//
// It answers Can checks over Subject, action, and Resource with a Decision of
// allow, implicit_deny, or explicit_deny. It is not authentication, and an
// error means infrastructure failure rather than denial.
//
// Type safety: Checker plus typed Options plus Adapter enum plus Factory. Rule,
// Subject, Resource, and Decision are typed, Effect is Allow or Deny, and roles
// map to inherited roles. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env PERMISSION_<FIELD> (no prefix, e.g. PERMISSION_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Permission(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Checker is stateless and exposes no Close; model and policy files load once at Open.
//
// Errors: sentinel errors, errors.Is compatible, prefixed permission:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Callers deny on error (fail-closed), model and policy paths reject dot-dot traversal and must exist, role names cap at 256 characters.
//
// Performance: rule validation is linear in rule count, RBAC checks read in-memory maps, Casbin files load once at Open. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package permission
