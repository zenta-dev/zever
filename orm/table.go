package orm

// Table is a typed reference to entity T's backing SQL table. Its
// fields are unexported; the only way to construct one is NewTable, which
// schema codegen calls with the derived table name and column list.
type Table[T any] struct {
	name    string
	columns []string
}

// NewTable builds a Table bound to name/columns, for schema codegen to
// call with the derived table name and column list.
func NewTable[T any](name string, columns []string) Table[T] {
	cp := make([]string, len(columns))
	copy(cp, columns)

	return Table[T]{name: name, columns: cp}
}

// Name returns t's SQL table name.
func (t Table[T]) Name() string { return t.name }

// Columns returns t's column list, in the order a codegen'd Scan method
// reads them.
func (t Table[T]) Columns() []string {
	cp := make([]string, len(t.columns))
	copy(cp, t.columns)

	return cp
}
