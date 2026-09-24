package router

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/zenta-dev/zever/internal/registry"
)

// Router is the interface that HTTP router adapters must implement.
type Router interface {
	// Handle registers handler for method and pattern.
	Handle(method string, pattern string, handler http.HandlerFunc)
	// Group returns a route group for prefix with optional middlewares.
	Group(prefix string, middlewares ...func(http.Handler) http.Handler) Group
	// Use appends middlewares to the router.
	Use(middlewares ...func(http.Handler) http.Handler)
	// ServeHTTP serves HTTP requests directly.
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

// Factory creates a Router from the given Options.
type Factory func(opts Options) (Router, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(a Adapter) error { return &DuplicateAdapterError{Adapter: a} },
	func(a Adapter) error { return &UnknownAdapterError{Adapter: a} },
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
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Router for adapter using the registered Factory and opts.
// Validate is called before factory lookup so invalid options fail-closed.
func Open(adapter Adapter, opts Options) (Router, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	r, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("router: open %s: %w", adapter, err)
	}

	return r, nil
}
