// Package flag evaluates typed feature flags with swappable adapters.
//
// It resolves Bool, String, Int, and JSON values with per-request targeting context. It is not a config system or rollout analytics store; missing keys return fallbacks, never errors.
//
// Type safety: Flag plus EvalContext plus typed Options plus Adapter enum plus Factory. StaticOptions and FirebaseOptions carry per-backend settings; keys are validated for shape and length. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env FLAG_<FIELD> (no prefix, e.g. FLAG_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Flag(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Flag releases its client with Close() which takes no ctx; attach targeting with WithEvalContext.
//
// Errors: sentinel errors, errors.Is compatible, prefixed flag:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Key bytes are never echoed in errors, length only; service account paths stay in config.
//
// Performance: static adapter loads JSON once with optional reload; remote calls use the default 30s timeout. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package flag
