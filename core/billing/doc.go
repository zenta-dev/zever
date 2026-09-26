// Package billing provides customers, subscriptions and invoices with swappable adapters.
//
// Billing is the subscription/invoice facade; payment is the separate
// charge/refund/webhook facade. Billing settles no real charges; provider
// adapters own transport.
//
// It owns the Billing facade plus minor-unit amount parsing and currency
// exponents. It settles no real charges; provider adapters own transport.
//
// Type safety: Billing plus typed Options plus Adapter enum plus Factory.
// Customer, Subscription, Invoice, SubscriptionStatus and InvoiceStatus are
// typed; ParseMinorUnits handles currency precision. Unsupported features fail
// closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests. Config file plus env BILLING_<FIELD> (no
// prefix, e.g. BILLING_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Billing(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources;
// only resolved services close. Billing closes with Close() error and no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed billing:. Name
// service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// SecretKey and APIKey values never appear in errors or logs.
//
// Performance: DefaultHTTPTimeout bounds provider calls; stub keeps maps
// behind a single mutex with ULID identifiers. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package billing
