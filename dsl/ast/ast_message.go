package ast

import "github.com/zenta-dev/zever/internal/dsl/diag"

// MessageDecl represents a message declaration.
type MessageDecl struct {
	Pos     diag.Position
	NamePos diag.Position
	Name    string
	Fields  []*FieldDecl
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *MessageDecl) declPos() diag.Position { return d.Pos }
