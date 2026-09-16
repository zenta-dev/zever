package postgres

// SupportsCTEMaterialized reports whether this Postgres server version
// supports the `MATERIALIZED` / `NOT MATERIALIZED` modifier on a common
// table expression (`WITH name AS MATERIALIZED (...)`), which arrived in
// Postgres 12. New targets a modern server (16.0), so the common path
// reports true.
func (d Dialect) SupportsCTEMaterialized() bool {
	return d.version.atLeast(12, 0, 0)
}

// SupportsCTESearchCycle reports whether this Postgres server version
// supports the recursive-CTE `SEARCH` and `CYCLE` clauses, which arrived
// together in Postgres 14. New targets a modern server (16.0), so the
// common path reports true.
func (d Dialect) SupportsCTESearchCycle() bool {
	return d.version.atLeast(14, 0, 0)
}
