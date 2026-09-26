package ir

import "github.com/zenta-dev/zever/dsl/diag"

// Schedule represents a scheduled job trigger in the schema.
type Schedule struct {
	Name         string
	Cron         string
	Module       *Module // Resolved pointer
	Dispatch     *Job    // The job to dispatch
	DispatchArgs []any   // Arguments to pass to the job
	Pos          diag.Position
	// DocComment is the "//" comment written immediately above this
	// schedule's declaration in the source .zen file, or "" if there was
	// none.
	DocComment string
}
