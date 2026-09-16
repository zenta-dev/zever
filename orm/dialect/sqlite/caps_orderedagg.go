package sqlite

// SupportsArrayAgg reports that SQLite supports `array_agg`. No SQLite
// version has it.
func (Dialect) SupportsArrayAgg() bool { return false }

// SupportsStringAgg reports that SQLite supports `string_agg`. No SQLite
// version has it; the SQLite spelling of the same concatenation is
// group_concat.
func (Dialect) SupportsStringAgg() bool { return false }

// SupportsGroupConcat reports that SQLite supports `group_concat(expr
// [, delim])`. It has had the function at every supported version.
func (Dialect) SupportsGroupConcat() bool { return true }

// SupportsOrderedAggregates reports whether this SQLite library version
// supports an ORDER BY inside an aggregate function call, which arrived in
// SQLite 3.44.0. New defaults to 3.46.0, so the common path reports true;
// an older version reports false for the ordered form while the unordered
// group_concat still renders.
func (d Dialect) SupportsOrderedAggregates() bool {
	return d.version.atLeast(3, 44, 0)
}
