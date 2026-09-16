package sqlite

// SupportsLateral reports that SQLite supports a LATERAL derived-table
// join. There is no LATERAL keyword in the SQLite grammar at any version,
// so this always reports false.
func (Dialect) SupportsLateral() bool { return false }
