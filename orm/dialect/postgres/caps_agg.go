package postgres

// SupportsAggregateFilter reports that Postgres supports the
// `agg(...) FILTER (WHERE ...)` clause on an aggregate. It is native
// Postgres syntax and available at every supported version.
func (Dialect) SupportsAggregateFilter() bool { return true }

// SupportsRollup reports that Postgres supports `GROUP BY ROLLUP(...)`.
func (Dialect) SupportsRollup() bool { return true }

// SupportsCube reports that Postgres supports `GROUP BY CUBE(...)`.
func (Dialect) SupportsCube() bool { return true }

// SupportsGroupingSets reports that Postgres supports
// `GROUP BY GROUPING SETS (...)`.
func (Dialect) SupportsGroupingSets() bool { return true }
