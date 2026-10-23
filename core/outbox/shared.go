package outbox

import (
	coredb "github.com/zenta-dev/zever/core/db"
)

// SharedFactory builds a Store over an already-open pool instead of opening
// its own connection. The caller retains pool ownership: Close on the built
// Store never closes conn. DB-backed adapters register their OpenFromDB
// constructors here so the container can borrow a shared pool without
// importing the adapter.
type SharedFactory func(conn coredb.DB, opts Options) (Store, error)
