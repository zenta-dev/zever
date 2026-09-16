package postgres

// SupportsLateral reports that Postgres supports a LATERAL derived-table
// join. LATERAL was introduced in Postgres 9.3 and is available at every
// version this package supports, so this is unconditional.
func (Dialect) SupportsLateral() bool { return true }
