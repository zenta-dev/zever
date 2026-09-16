package orm

// Row is the minimal interface a codegen'd Scan method needs: db.Rows
// already satisfies this structurally (it has Scan plus other methods this
// interface doesn't require), so a db.Rows value is assignable to an
// orm.Row-typed parameter with no adapter wrapper.
type Row interface {
	Scan(dest ...any) error
}

// Scanner is implemented by a codegen'd entity pointer type: it reads one
// row's columns, in the order its Table's Columns() lists them, into the
// receiver.
type Scanner interface {
	Scan(row Row) error
}
