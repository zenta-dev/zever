package session_test

import (
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

// ExampleOpen opens the in-memory session store with default TTL.
func ExampleOpen() {
	_ = session.Register(session.Memory, sessionmemory.New)

	s, err := session.Open(session.Memory, session.Options{})
	if err != nil {
		return
	}

	defer func() { _ = s.Close() }()
}
