// Package dialect defines the SQL-dialect abstraction the query renderer
// depends on, plus the optional capability sub-interfaces gating every
// feature beyond plain SELECT/WHERE/ORDER/LIMIT.
//
// A capability is checked via a type assertion at query-build time. A
// dialect lacking one yields the typed ErrUnsupportedByDialect, never a
// silent wrong-SQL fallback or a panic.
//
// The in-tree dialects are sqlite and postgres, resolved by name through
// For. Out-of-tree dialect packages register themselves via Register.
//
// A rule for future db-backed adapters: when the driver they open on lacks
// a required capability, Open (via ensureSchema) returns the capability
// error wrapping ErrUnsupportedByDialect, never a silently-degraded driver.
package dialect
