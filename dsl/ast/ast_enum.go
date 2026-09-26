package ast

import "github.com/zenta-dev/zever/dsl/diag"

// EnumDecl represents a top-level named enum declaration:
// enum Name { value1, value2, ... }.
type EnumDecl struct {
	Pos      diag.Position
	NamePos  diag.Position
	Name     string
	Values   []string
	ValuePos []diag.Position
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *EnumDecl) declPos() diag.Position { return d.Pos }
