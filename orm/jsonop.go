package orm

// JSONStep is one step of a JSON path into a JSON document: an object key
// or an array index. It is the typed building block the orm/json/* packages
// use to describe where inside a JSON column an operator should look, so a
// path is never a caller-supplied SQL fragment -- only data, escaped and
// quoted by orm/render at render time.
type JSONStep struct {
	Key     string
	Index   int64
	IsIndex bool
}

// JSONKey builds a JSONStep addressing an object key.
func JSONKey(key string) JSONStep { return JSONStep{Key: key} }

// JSONIndex builds a JSONStep addressing an array element.
func JSONIndex(index int64) JSONStep { return JSONStep{Index: index, IsIndex: true} }

// JSONOp identifies a JSON operator applied to a JSON column. The value is
// abstract -- "extract text at this path", "contains this document", etc. --
// and orm/render turns it into the concrete per-dialect syntax: jsonb
// operators (`->`, `->>`, `@>`, `?`, `#>`, `#>>`) on Postgres, json1
// functions (`json_extract`, `json_type`, `json_each`) on SQLite (see
// capability table, and render/json.go).
type JSONOp int

// Supported JSON operators.
const (
	// JSONExtract is a jsonb-valued extraction (`col->'key'` / json1's
	// `json_extract`), used where the extracted value is still JSON.
	JSONExtract JSONOp = iota
	// JSONExtractText is a text-valued extraction (`col->>'key'` / json1's
	// `json_extract`), used where the extracted value is compared as text.
	JSONExtractText
	// JSONPathExtract is a jsonb-valued path extraction (`col#>'{a,b}'`).
	JSONPathExtract
	// JSONPathExtractText is a text-valued path extraction (`col#>>'{a,b}'`).
	JSONPathExtractText
	// JSONContains is a jsonb containment predicate (`col @> 'doc'`),
	// Postgres-only; sqlite json1 has no containment operator (the sqlite
	// package exposes JSONArrayContains via json_each instead).
	JSONContains
	// JSONKeyExists is a key-existence predicate (`col ? 'key'` on Postgres,
	// `json_extract(col,'$.key') IS NOT NULL` on SQLite).
	JSONKeyExists
	// JSONType is a value-type function (`jsonb_typeof` / `json_type`).
	JSONType
	// JSONArrayContains is a "the JSON array contains this element"
	// predicate, rendered on SQLite as
	// `EXISTS (SELECT 1 FROM json_each(col) WHERE value = ?)`.
	JSONArrayContains
	// JSONLength is a value-length function (`JSON_LENGTH`). No in-tree
	// dialect package builds JSONLength nodes; it is reserved for
	// out-of-tree dialects.
	JSONLength
	// JSONPathExists is a jsonpath existence predicate (`col @? $n`),
	// Postgres-only: does the SQL/JSON path expression (a bound jsonpath
	// string) yield at least one item for the JSON value? The path is
	// always a bound argument, never inlined. Built by postgres.PathExists.
	JSONPathExists
	// JSONPathMatch is a jsonpath match predicate (`col @@ $n`),
	// Postgres-only: does the SQL/JSON path predicate (a bound jsonpath
	// string) evaluate to true for the JSON value? Built by
	// postgres.PathMatch.
	JSONPathMatch
	// JSONPathExistsFunc is the function form of jsonpath existence
	// (`jsonb_path_exists(col, $n)`), Postgres-only. Built by
	// postgres.PathExistsFunc.
	JSONPathExistsFunc
	// JSONPathMatchFunc is the function form of jsonpath matching
	// (`jsonb_path_match(col, $n)`), Postgres-only. Built by
	// postgres.PathMatchFunc.
	JSONPathMatchFunc
	// JSONPathQueryFirst is a jsonb-valued expression yielding the first
	// item matched by a jsonpath (`jsonb_path_query_first(col, $n)`),
	// Postgres-only and comparable against a JSON document. Built by
	// postgres.QueryFirst.
	JSONPathQueryFirst
	// JSONKeyExistsAny is a "the document has any of these keys" predicate
	// (`col ?| ARRAY[$1, $2]`), Postgres-only. Node.Value is always a
	// []string. Built by postgres.KeyExistsAny.
	JSONKeyExistsAny
	// JSONKeyExistsAll is a "the document has all of these keys" predicate
	// (`col ?& ARRAY[$1, $2]`), Postgres-only. Node.Value is always a
	// []string. Built by postgres.KeyExistsAll.
	JSONKeyExistsAll
)

// JSONExpr is the erased JSON expression an NJSON predicate node carries:
// which operator, applied along which path. The base column lives on the
// surrounding Node (Table/Column), exactly as a plain binary predicate's
// column does. Path carries the bound jsonpath text for the Postgres-only
// jsonpath operators (JSONPathExists/JSONPathMatch and the jsonb_path_*
// function forms), whose operand is a jsonpath string rather than the
// node's comparison value -- keeping the two independent (QueryFirst needs
// a jsonpath AND a comparison document).
type JSONExpr struct {
	Op    JSONOp
	Steps []JSONStep
	Path  string
}

// NewJSONPredicate wraps a JSON expression over table.column into a typed
// Predicate. cmp is the comparison to apply to the expression's result
// (Eq/Neq/Gt/... for extraction-style operators; ignored for the
// self-contained JSONContains/JSONKeyExists/JSONArrayContains predicates,
// which bind value directly). It is exported for the orm/json/* dialect
// packages to build predicates from their typed helpers; ordinary callers
// use those packages' fluent surfaces, never this constructor directly.
func NewJSONPredicate[T any](table, column string, expr JSONExpr, cmp Op, value any) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NJSON, Table: table, Column: column, JSON: &expr, Op: cmp, Value: value}}
}
