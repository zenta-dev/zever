package router

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Router is the interface that HTTP router adapters must implement.
type Router interface {
	// Handle registers handler for method and pattern.
	Handle(method string, pattern string, handler http.HandlerFunc)
	// Group returns a route group for prefix with optional middlewares.
	Group(prefix string, middlewares ...func(http.Handler) http.Handler) Group
	// Use appends middlewares to the router.
	Use(middlewares ...func(http.Handler) http.Handler)
	http.Handler
}

// Group represents a route group with middleware.
type Group interface {
	// Handle registers handler for method and pattern within the group.
	Handle(method string, pattern string, handler http.HandlerFunc)
	// Group returns a nested route group for prefix with optional middlewares.
	Group(prefix string, middlewares ...func(http.Handler) http.Handler) Group
	// Use appends middlewares to the group.
	Use(middlewares ...func(http.Handler) http.Handler)
}

// Factory creates a Router from typed options.
type Factory func(opts Options) (Router, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// standardMethods lists the recognized HTTP methods.
var standardMethods = map[string]bool{
	http.MethodConnect: true,
	http.MethodDelete:  true,
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodPatch:   true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodTrace:   true,
}

// ValidMethod reports whether method is a recognized HTTP method.
func ValidMethod(method string) bool {
	return standardMethods[strings.ToUpper(strings.TrimSpace(method))]
}

// Register associates an Adapter with a Factory for later use by Open.
func Register(a Adapter, f Factory) error {
	if f == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, a)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[a]; dup {
		return &DuplicateAdapterError{Adapter: a}
	}

	factories[a] = f

	return nil
}

// Open creates a Router for adapter using the registered Factory and opts.
// Validate is called before factory lookup so invalid options fail-closed.
func Open(a Adapter, opts Options) (Router, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[a]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: a}
	}

	r, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("router: open %s: %w", a, err)
	}

	return r, nil
}
