package router

import (
	"net/http"
	"strings"
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
