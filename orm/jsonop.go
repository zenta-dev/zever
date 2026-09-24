package orm

import "github.com/zenta-dev/zever/orm/render"

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
type JSONOp = render.JSONOp

// Supported JSON operators. These re-export render's identically-named
// constants; see render.JSONOp's constants for the full per-value
// documentation (which operator/function each renders to per dialect).
const (
	JSONExtract         = render.JSONExtract
	JSONExtractText     = render.JSONExtractText
	JSONPathExtract     = render.JSONPathExtract
	JSONPathExtractText = render.JSONPathExtractText
	JSONContains        = render.JSONContains
	JSONKeyExists       = render.JSONKeyExists
	JSONType            = render.JSONType
	JSONArrayContains   = render.JSONArrayContains
	JSONLength          = render.JSONLength
	JSONPathExists      = render.JSONPathExists
	JSONPathMatch       = render.JSONPathMatch
	JSONPathExistsFunc  = render.JSONPathExistsFunc
	JSONPathMatchFunc   = render.JSONPathMatchFunc
	JSONPathQueryFirst  = render.JSONPathQueryFirst
	JSONKeyExistsAny    = render.JSONKeyExistsAny
	JSONKeyExistsAll    = render.JSONKeyExistsAll
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
