// Package workflow runs durable workflow executions with swappable adapters.
//
// It starts runs, delivers signals, evaluates queries, and cancels runs to shutdown. It is not a cron scheduler and does not fire on time specs itself.
//
// Type safety: Workflow plus typed Options plus Adapter enum plus Factory. RunID is a typed string; Options carry HostPort plus Namespace with host port validation. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env WORKFLOW_<FIELD> (no prefix, e.g. WORKFLOW_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Workflow(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error with no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed workflow:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. HostPort is validated as host and port and run IDs are opaque strings.
//
// Performance: memory backend is process-local with bounded pending buffers reporting ErrPendingFull. Bounded pools and timeouts via backend defaults.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package workflow
