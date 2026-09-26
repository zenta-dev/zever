package ast

import "github.com/zenta-dev/zever/dsl/diag"

// ScheduleDecl represents a scheduled job declaration.
type ScheduleDecl struct {
	Pos      diag.Position
	NamePos  diag.Position
	CronPos  diag.Position
	Name     string
	Cron     string
	Dispatch Value
	// DocComment holds the "//" comment(s) written immediately above this
	// declaration, if any -- see parser.docCommentFor. Empty when there is
	// none.
	DocComment string
}

func (d *ScheduleDecl) declPos() diag.Position { return d.Pos }
