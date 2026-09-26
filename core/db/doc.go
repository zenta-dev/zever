// Package db queries and transacts over SQL pools with swappable adapters.
//
// It runs queries through Query and Exec and groups work in Tx via WithTx for sqlite and postgres. It is not a query builder or migration runner; use orm for typed queries.
//
// Type safety: DB plus Rows plus Stmt plus Preparer plus Tx plus Transactor plus typed Options plus Adapter enum plus Factory. Options is a union of DSN, pool bounds, lifetimes, and Path; Dialect reports the SQL dialect per call. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env DB_<FIELD> (no prefix, e.g. DB_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.DB(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. DB closes with Close(ctx); Rows and Stmt each have Close; Tx ends with Commit or Rollback, and WithTx commits on success and rolls back on error or panic.
//
// Errors: sentinel errors, errors.Is compatible, prefixed db:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. DSN may carry credentials and path content never appears in errors; validate pool bounds before opening.
//
// Performance: database/sql pools bounded by MaxConns with MaxConnLifetime and MaxConnIdleTime; :memory: sqlite pins to one connection. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package db
