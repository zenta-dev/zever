package sqlite

// SupportsAggregateFilter reports whether this SQLite library version
// supports the `agg(...) FILTER (WHERE ...)` clause, which arrived in
// SQLite 3.30.0. Older versions reject the syntax. New defaults to 3.46.0,
// so the common path reports true.
func (d Dialect) SupportsAggregateFilter() bool {
	return d.version.atLeast(3, 30, 0)
}

// SupportsRollup reports that SQLite supports `GROUP BY ROLLUP(...)`. No
// SQLite version has the construct, so this always reports false.
func (Dialect) SupportsRollup() bool { return false }

// SupportsCube reports that SQLite supports `GROUP BY CUBE(...)`. No SQLite
// version has the construct, so this always reports false.
func (Dialect) SupportsCube() bool { return false }

// SupportsGroupingSets reports that SQLite supports
// `GROUP BY GROUPING SETS (...)`. No SQLite version has the construct, so
// this always reports false.
func (Dialect) SupportsGroupingSets() bool { return false }
