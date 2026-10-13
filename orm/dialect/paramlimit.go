package dialect

// Bind-parameter budgets per statement. Drivers and servers reject a
// statement whose placeholder count exceeds these, so the ORM's IN-list
// fan-out splits value lists to stay under the resolved dialect's budget:
//
//   - Postgres addresses bind parameters with a uint16 on the wire, so a
//     statement carries at most 65535 parameters.
//   - SQLite caps host parameters at SQLITE_MAX_VARIABLE_NUMBER, which
//     defaults to 999 before SQLite 3.32.0 and 32766 after (raised in
//     3.32.0, 2020-05-22; see sqlite.org/limits.html). A custom SQLite
//     build may raise the cap further at compile time, but the default is
//     the only portable assumption.
const (
	// MaxPostgresParams is the per-statement parameter budget for Postgres.
	MaxPostgresParams = 65535
	// MaxSQLiteParamsModern is the default per-statement budget for
	// SQLite >= 3.32.0.
	MaxSQLiteParamsModern = 32766
	// MaxSQLiteParamsLegacy is the default per-statement budget for
	// SQLite < 3.32.0.
	MaxSQLiteParamsLegacy = 999
	// DefaultMaxParams is the conservative budget for a dialect that does
	// not report its own: the lowest portable ceiling, so an unknown
	// backend fails safe rather than over-allocating placeholders.
	DefaultMaxParams = 999
)

// ParamLimitDialect is implemented by dialects that report a per-statement
// bind-parameter budget. It is a separate interface (rather than a method
// on Dialect) so existing Dialect implementations keep compiling and only
// dialects with a known ceiling opt in; MaxParams falls back to
// DefaultMaxParams for the rest.
type ParamLimitDialect interface {
	Dialect
	MaxParams() int
}

// MaxParams returns d's per-statement bind-parameter budget: the
// ParamLimitDialect value when d implements it (and reports positive), or
// DefaultMaxParams otherwise. Callers test chunking against this, never
// against a hardcoded constant.
func MaxParams(d Dialect) int {
	if pl, ok := d.(ParamLimitDialect); ok {
		if n := pl.MaxParams(); n > 0 {
			return n
		}
	}

	return DefaultMaxParams
}
