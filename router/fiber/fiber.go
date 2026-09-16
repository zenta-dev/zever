package fiber

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/zenta-dev/zever/router"
)

// New creates a router.Router backed by Fiber v2.
//
// An empty AppName defaults to "app". Invalid options fail closed.
func New(opts router.Options) (router.Router, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("fiber: %w", err)
	}

	appName := opts.AppName
	if appName == "" {
		appName = "app"
	}

	app := fiber.New(fiber.Config{AppName: appName, CaseSensitive: true})
	app.Use(recovermw.New())

	return &fiberDriver{app: app, routes: make(map[string]bool)}, nil
}

type fiberDriver struct {
	app    *fiber.App
	mu     sync.Mutex
	routes map[string]bool
}

func (d *fiberDriver) Handle(method, pattern string, handler http.HandlerFunc) {
	m, p, ok := d.checkRoute(method, pattern)
	if !ok {
		return
	}

	if err := safeAdd(d.app, m, p, wrapHandler(handler)); err != nil {
		routerLogf("[router] fiber: failed to register route %s %s: %v", m, p, err)

		return
	}

	d.markRoute(m, p)
}

func (d *fiberDriver) checkRoute(method, pattern string) (string, string, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if !router.ValidMethod(method) {
		routerLogf("[router] fiber: skipping route with unsupported method %q (%s)", method, pattern)

		return "", "", false
	}

	norm, err := convertAndNormalize(pattern)
	if err != nil {
		return "", "", false
	}

	pattern = norm
	if pattern == "" {
		return "", "", false
	}

	key := routeKey(method, pattern)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.routes[key] {
		routerLogf("[router] fiber: skipping duplicate route %s %s", method, pattern)

		return "", "", false
	}

	return method, pattern, true
}

func (d *fiberDriver) markRoute(method, pattern string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.routes[routeKey(method, pattern)] = true
}

func (d *fiberDriver) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	norm, err := convertAndNormalize(prefix)
	if err != nil {
		// convertAndNormalize already logged; fallback to normalized raw prefix
		norm = router.NormalizePattern(prefix)
	}

	prefix = norm

	fg := d.app.Group(prefix)
	for _, mw := range middlewares {
		fg.Use(adaptor.HTTPMiddleware(mw))
	}

	return &fiberGroup{d: d, group: fg, prefix: prefix}
}

func (d *fiberDriver) Use(middlewares ...func(http.Handler) http.Handler) {
	for _, mw := range middlewares {
		d.app.Use(adaptor.HTTPMiddleware(mw))
	}
}

func (d *fiberDriver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	adaptor.FiberApp(d.app).ServeHTTP(w, r)
}

type fiberGroup struct {
	d      *fiberDriver
	group  fiber.Router
	prefix string
}

func (g *fiberGroup) Handle(method, pattern string, handler http.HandlerFunc) {
	norm, err := convertAndNormalize(pattern)
	if err != nil {
		return
	}

	p := router.NormalizePattern(norm)
	if m, _, ok := g.d.checkRoute(method, g.fullPath(p)); ok {
		if err := safeAdd(g.group, m, p, wrapHandler(handler)); err != nil {
			routerLogf("[router] fiber: failed to register route %s %s: %v", m, p, err)

			return
		}

		g.d.markRoute(m, g.fullPath(p))
	}
}

func (g *fiberGroup) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	norm, err := convertAndNormalize(prefix)
	if err != nil {
		// convertAndNormalize already logged; fallback to normalized raw prefix
		norm = router.NormalizePattern(prefix)
	}

	prefix = router.NormalizePattern(norm)

	fg := g.group.Group(prefix)
	for _, mw := range middlewares {
		fg.Use(adaptor.HTTPMiddleware(mw))
	}

	return &fiberGroup{d: g.d, group: fg, prefix: g.fullPath(prefix)}
}

func (g *fiberGroup) fullPath(pattern string) string {
	return router.NormalizePattern(strings.TrimRight(g.prefix, "/") + "/" + strings.TrimLeft(pattern, "/"))
}

func (g *fiberGroup) Use(middlewares ...func(http.Handler) http.Handler) {
	for _, mw := range middlewares {
		g.group.Use(adaptor.HTTPMiddleware(mw))
	}
}

func convertAndNormalize(pattern string) (string, error) {
	converted, err := router.ConvertColonRegex(pattern)
	if err != nil {
		routerLogf("[router] fiber: malformed pattern %q: %v", pattern, err)

		return "", err
	}

	return router.NormalizePattern(converted), nil
}

func routeKey(method, pattern string) string {
	return method + " " + strings.TrimRight(pattern, "/")
}

func safeAdd(r fiber.Router, method, pattern string, handler fiber.Handler) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			err = fmt.Errorf("[router] fiber: route registration panicked for %s %s: %v\n%s", method, pattern, rec, stack)
		}
	}()

	r.Add(method, pattern, handler)

	return nil
}

func routerLogf(format string, args ...any) {
	log.Printf(format, args...) //nolint:forbidigo
}

func wrapHandler(handler http.HandlerFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return adaptor.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(w, r.WithContext(router.WithParams(r.Context(), c.AllParams())))
		}))(c)
	}
}
