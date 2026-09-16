package postgres

// SupportsArrayAgg reports that Postgres supports `array_agg(expr [ORDER BY
// ...])`. It is native Postgres syntax at every supported version.
func (Dialect) SupportsArrayAgg() bool { return true }

// SupportsStringAgg reports that Postgres supports `string_agg(expr, delim
// [ORDER BY ...])`. It is native Postgres syntax at every supported version.
func (Dialect) SupportsStringAgg() bool { return true }

// SupportsGroupConcat reports that Postgres supports group_concat. No
// Postgres version has that function; the Postgres spelling of the same
// concatenation is string_agg.
func (Dialect) SupportsGroupConcat() bool { return false }

// SupportsOrderedAggregates reports that Postgres supports an ORDER BY inside
// an aggregate function call. It is native syntax at every supported version.
func (Dialect) SupportsOrderedAggregates() bool { return true }
