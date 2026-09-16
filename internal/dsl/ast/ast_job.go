package ast

import "github.com/zenta-dev/zever/internal/dsl/diag"

// JobDecl represents a background job declaration.
type JobDecl struct {
	Pos      diag.Position
	NamePos  diag.Position
	QueuePos diag.Position
	Name     string
	Queue    string
	Params   []*ParamDecl
	Retry    []Value
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *JobDecl) declPos() diag.Position { return d.Pos }
