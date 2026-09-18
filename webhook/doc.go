// Package webhook delivers event fan-out with swappable adapters.
//
// It registers per-target secrets, unregisters subscriptions, and delivers payloads to event targets. It is not a queue itself though one backend rides on queue.
//
// Type safety: Webhook plus typed Options plus Adapter enum plus Factory. Options carry Timeout plus MaxRetries plus QueueAdapter plus QueueOpts plus DeadLetterTopic plus DSN plus ReplayTolerance; there is no shared APIKey by design. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env WEBHOOK_<FIELD> (no prefix, e.g. WEBHOOK_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Webhook(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close() error with no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed webhook:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. Endpoint secrets are per target at Register time and AllowPrivateTargets stays false in production. Outbound signatures embed a timestamp ("t=<unix-timestamp>,v1=<hex-hmac>" over "<unix-timestamp>.<payload>"); verifiers check the HMAC with hmac.Equal and separately reject timestamps outside ReplayTolerance (default 5 minutes), guarding against replay.
//
// Performance: per-delivery Timeout with MaxRetries after the first attempt and dead-letter routing on exhaustion. Bounded pools and timeouts via Options.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package webhook
