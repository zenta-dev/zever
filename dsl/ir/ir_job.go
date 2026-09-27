package ir

import (
	"time"

	"github.com/zenta-dev/zever/dsl/diag"
)

// RetryPolicy represents retry configuration for a job.
// MaxAttempts: 1 == "no retry declared".
type RetryPolicy struct {
	MaxAttempts int           // Maximum number of attempts
	Backoff     string        // Backoff strategy
	Base        time.Duration // Base backoff duration
	Pos         diag.Position // Source position
}

// Job represents a background job in the schema.
// Queue defaults to "default".
type Job struct {
	Name   string
	Module *Module // Resolved pointer
	Params []*Param
	Queue  string
	Retry  RetryPolicy
	Pos    diag.Position
	// DocComment is the "//" comment written immediately above this job's
	// declaration in the source .zen file, or "" if there was none.
	DocComment string
}
