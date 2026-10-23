// Package payment payment-processing facade with swappable adapters.
//
// Payment is the charge/refund/webhook facade; billing is the separate
// subscription/invoice facade. It creates, refunds, fetches, and webhook-decodes payments through Payment
// with minor-unit amounts and typed Request, Result, and Event values. It is
// not a ledger of record and it never stores card data; backends own that.
//
// Type safety: Payment plus typed Options plus Factory. Status,
// method, and event types are strings with fixed constants, and LimitDecode
// bounds webhook JSON. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Payment(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Payment exposes Close() error for backend cleanup.
//
// Errors: sentinel errors, errors.Is compatible, prefixed payment:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. SecretKey, APIKey, and WebhookSecret stay out of logs, webhooks verify signatures under a size bound, and custom endpoints require scheme plus host.
//
// Performance: DefaultMaxWebhookBytes is 1 MiB, DefaultHTTPTimeout is 30 seconds, amounts are int64 minor units. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package payment
