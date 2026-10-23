// Package geo resolves places and distances with swappable adapters.
//
// It geocodes addresses, reverse-geocodes coordinates, and measures distance in meters. It is not a map renderer or routing engine; results are points and formatted names.
//
// Type safety: Geo plus typed Options plus Factory. Location, Address, and Point are typed results and ValidCoord checks WGS84 range and finiteness. Unsupported features fail closed.
//
// DX: construct backends directly with the adapter package's New constructor. Backend selection is compile-time: importing the adapter wires it. Options are typed with zero-infra defaults for tests. Configuration comes from the environment only: .env, .env.local, and the process environment, with keys named <SERVICE>_<FIELD> (no prefix). See config/README.md.
//
// Container: container.New(cfg) then c.Geo(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. Geo releases its backend with Close() which takes no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed geo:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. APIKey is never logged, https is required unless AllowInsecure is set for tests, and OSM needs a User-Agent.
//
// Performance: request timeouts bound HTTP calls and MaxResponseBody caps response bodies; static adapter serves a file-backed list from memory. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see Example in example_test.go.
package geo
