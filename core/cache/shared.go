package cache

import (
	coredb "github.com/zenta-dev/zever/core/db"
)

// SharedFactory builds a Cache over an already-open pool instead of opening
// its own connection. The caller retains pool ownership: Close on the built
// Cache never closes conn. DB-backed backends expose their
// OpenFromDB constructors here so the container can borrow a shared pool
// without importing the backend package. Core importing core/db mirrors the
// core/scheduler to core/job precedent: a layering dependency, never
// a new interface method.
type SharedFactory func(conn coredb.DB, opts Options) (Cache, error)
