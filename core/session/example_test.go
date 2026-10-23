package session_test

import (
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

// Example opens the in-memory session store with default TTL.
func Example() {
	s, err := sessionmemory.New(session.Options{})
	if err != nil {
		return
	}

	defer func() { _ = s.Close() }()
}
