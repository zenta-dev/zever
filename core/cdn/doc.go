// Package cdn invalidates cached content at a CDN provider's edge with swappable adapters.
//
// It purges by URL, cache tag, or the whole zone/distribution. The noop adapter discards purges; the Cloudflare adapter calls the Cloudflare cache-purge API.
//
// Type safety: CDN plus typed Options plus Factory. Options carry APIToken, ZoneID, and BaseURL; PurgeRequest describes what to invalidate and unsupported shapes fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: import the adapter package and wire its New constructor with container.Provide. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.CDN(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Close shape is Close(ctx context.Context) error.
//
// Errors: sentinel errors, errors.Is compatible, prefixed cdn:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. API tokens are never logged.
//
// Performance: Purge batches URLs or tags in a single provider call. Per-request timeouts come from ctx.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
package cdn
