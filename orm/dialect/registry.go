package dialect

import (
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

var (
	mu sync.RWMutex
	// factories resolves a dialect name to a fresh Dialect value. It starts
	// with the two in-tree dialects; Register adds more.
	factories = map[string]func() Dialect{
		"sqlite":   func() Dialect { return sqlite.New() },
		"postgres": func() Dialect { return postgres.New() },
	}
)

// For turns a DB Dialect name into the Dialect query methods render SQL
// with. For never silently falls back for an unrecognized name; an unknown
// name returns a clear error instead of a silently-wrong default.
func For(name string) (Dialect, error) {
	mu.RLock()

	f, ok := factories[name]

	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownDialect, name)
	}

	return f(), nil
}

// Register adds a Dialect factory under name for For to resolve. It exists
// so tests can inject a mock dialect implementing only the base Dialect
// interface, and for out-of-tree dialect packages to register themselves.
// It returns an error (never panics) for an empty name, a nil factory, or
// a name that is already registered.
func Register(name string, factory func() Dialect) error {
	if name == "" {
		return ErrEmptyName
	}

	if factory == nil {
		return ErrNilFactory
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		return fmt.Errorf("%w %q", ErrDuplicate, name)
	}

	factories[name] = factory

	return nil
}
