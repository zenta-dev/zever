package scheduler

import (
	coredb "github.com/zenta-dev/zever/core/db"
)

// SharedFactory builds a Scheduler over an already-open pool instead of
// opening its own connection. The caller retains pool ownership: Close on
// the built Scheduler never closes conn. Adapters with a DB-backed backend
// expose their OpenFromDB constructors here so the container can borrow a
// shared pool without importing the backend package. Core importing core/db mirrors
// the existing core/scheduler to core/job precedent: a layering
// dependency, never a new interface method.
type SharedFactory func(conn coredb.DB, opts Options) (Scheduler, error)
