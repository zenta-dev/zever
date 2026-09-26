package ast

import "github.com/zenta-dev/zever/internal/dsl/diag"

// ServiceDecl represents a service declaration.
type ServiceDecl struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	RPCs    []*RPCDecl
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *ServiceDecl) declPos() diag.Position { return d.Pos }

// ParamDecl represents a parameter declaration in an RPC.
type ParamDecl struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Type    *TypeExpr
	// Attributes holds any `@attribute(...)` annotations on the param, e.g.
	// `@validate(format: "email")`. Reuses the same generic attribute-list
	// grammar entity fields already use (parseAttributeList).
	Attributes []*Attribute
}

// RPCDecl represents a remote procedure call declaration.
type RPCDecl struct {
	Pos        diag.Position
	NamePos    diag.Position
	ReturnsPos diag.Position
	Name       string
	Returns    string
	Params     []*ParamDecl
	// HTTP/Auth/Permission each hold only the last occurrence when the
	// corresponding http:/auth:/permission: option is written more than
	// once in one rpc body — a known v1 gap: repeating one of these
	// options is not rejected, it silently keeps the last value parsed.
	HTTP       *HTTPOption
	Auth       Value
	Permission Value
	// Errors holds one item per entry in an errors: {...} set, in
	// declaration order. Each item is either an *IdentValue (a bare error
	// code) or a *CallValue (a code with an optional positional string
	// message), the same shapes resolvePermission already destructures.
	// A repeated errors: option in one rpc body has the same v1 gap as
	// HTTP/Auth/Permission above: it silently keeps the last occurrence
	// parsed.
	Errors []Value
	// Paginated is set by a `paginated: true` (or `paginated: false`)
	// rpc-body option. Absent defaults to false (a plain, single-entity
	// response), matching this DSL's existing "explicit boolean literal"
	// style rather than a bare presence-implies-true marker. Same v1 gap as
	// HTTP/Auth/Permission/Errors above: a repeated paginated: option
	// silently keeps the last occurrence parsed.
	Paginated bool
	// PaginatedPos is the source position of the paginated: option's value
	// token, valid only when PaginatedSet is true.
	PaginatedPos diag.Position
	// PaginatedSet reports whether a paginated: option was present at all,
	// distinguishing an explicit `paginated: false` from an absent option
	// (both resolve to Paginated == false).
	PaginatedSet bool
	// DocComment holds the "//" comment(s) written immediately above this
	// rpc, if any -- see parser.docCommentFor. Empty when there is none.
	DocComment string
}

// HTTPOption represents HTTP method and path configuration for an RPC.
type HTTPOption struct {
	Pos       diag.Position
	MethodPos diag.Position
	PathPos   diag.Position
	Method    string
	Path      string
}
