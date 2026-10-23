package workflow

import (
	coredb "github.com/zenta-dev/zever/core/db"
)

// SharedFactory builds a Workflow over an already-open pool instead of
// opening its own connection. The caller retains pool ownership: Close on
// the built Workflow never closes conn. Adapters with a DB-backed backend
// register their OpenFromDB constructors here so the container can borrow a
// shared pool without importing the adapter. Core importing core/db mirrors
// the core/scheduler to core/job precedent: a registry-level dependency,
// never a new interface method.
type SharedFactory func(conn coredb.DB, opts Options) (Workflow, error)
