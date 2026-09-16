package sqlite

// SupportsCTEMaterialized reports whether this SQLite library version
// supports the `MATERIALIZED` / `NOT MATERIALIZED` modifier on a common
// table expression (`WITH name AS MATERIALIZED (...)`), which arrived in
// SQLite 3.35.0. Older versions reject the keyword. New defaults to 3.46.0,
// so the common path reports true.
func (d Dialect) SupportsCTEMaterialized() bool {
	return d.version.atLeast(3, 35, 0)
}

// SupportsCTESearchCycle reports whether this SQLite library version
// supports the recursive-CTE `SEARCH` and `CYCLE` clauses. No SQLite version
// has them, so this always reports false.
func (Dialect) SupportsCTESearchCycle() bool { return false }
