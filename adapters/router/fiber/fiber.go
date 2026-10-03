package fiber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/router"
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

	return &fiberDriver{app: app, httpHandler: adaptor.FiberApp(app), routes: make(map[string]bool), logger: opts.Logger}, nil
}

type fiberDriver struct {
	app         *fiber.App
	httpHandler http.HandlerFunc
	mu          sync.Mutex
	routes      map[string]bool
	logger      log.Logger
}

func (d *fiberDriver) log() log.Logger {
	if d != nil && d.logger != nil {
		return d.logger
	}

	return noop.New()
}

func (d *fiberDriver) Handle(method, pattern string, handler http.HandlerFunc) {
	m, p, ok := d.checkRoute(method, pattern)
	if !ok {
		return
	}

	if err := safeAdd(d.app, m, p, wrapHandler(handler)); err != nil {
		d.routerLogf("[router] fiber: failed to register route %s %s: %v", m, p, err)

		return
	}

	d.markRoute(m, p)
}

func (d *fiberDriver) checkRoute(method, pattern string) (string, string, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if !router.ValidMethod(method) {
		d.routerLogf("[router] fiber: skipping route with unsupported method %q (%s)", method, pattern)

		return "", "", false
	}

	norm, err := d.convertAndNormalize(pattern)
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
		d.routerLogf("[router] fiber: skipping duplicate route %s %s", method, pattern)

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
	norm, err := d.convertAndNormalize(prefix)
	if err != nil {
		// convertAndNormalize already logged; fallback to normalized raw prefix
		norm = router.NormalizePattern(prefix)
	}

	prefix = norm

	fg := d.app.Group(prefix)
	for _, mw := range middlewares {
		fg.Use(wrapMiddleware(mw))
	}

	return &fiberGroup{d: d, group: fg, prefix: prefix}
}

func (d *fiberDriver) Use(middlewares ...func(http.Handler) http.Handler) {
	for _, mw := range middlewares {
		d.app.Use(wrapMiddleware(mw))
	}
}

func (d *fiberDriver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.httpHandler(w, r)
}

type fiberGroup struct {
	d      *fiberDriver
	group  fiber.Router
	prefix string
}

func (g *fiberGroup) Handle(method, pattern string, handler http.HandlerFunc) {
	norm, err := g.d.convertAndNormalize(pattern)
	if err != nil {
		return
	}

	p := router.NormalizePattern(norm)
	if m, _, ok := g.d.checkRoute(method, g.fullPath(p)); ok {
		if err := safeAdd(g.group, m, p, wrapHandler(handler)); err != nil {
			g.d.routerLogf("[router] fiber: failed to register route %s %s: %v", m, p, err)

			return
		}

		g.d.markRoute(m, g.fullPath(p))
	}
}

func (g *fiberGroup) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	norm, err := g.d.convertAndNormalize(prefix)
	if err != nil {
		// convertAndNormalize already logged; fallback to normalized raw prefix
		norm = router.NormalizePattern(prefix)
	}

	prefix = router.NormalizePattern(norm)

	fg := g.group.Group(prefix)
	for _, mw := range middlewares {
		fg.Use(wrapMiddleware(mw))
	}

	return &fiberGroup{d: g.d, group: fg, prefix: g.fullPath(prefix)}
}

func (g *fiberGroup) fullPath(pattern string) string {
	return router.NormalizePattern(strings.TrimRight(g.prefix, "/") + "/" + strings.TrimLeft(pattern, "/"))
}

func (g *fiberGroup) Use(middlewares ...func(http.Handler) http.Handler) {
	for _, mw := range middlewares {
		g.group.Use(wrapMiddleware(mw))
	}
}

func (d *fiberDriver) convertAndNormalize(pattern string) (string, error) {
	converted, err := router.ConvertColonRegex(pattern)
	if err != nil {
		d.routerLogf("[router] fiber: malformed pattern %q: %v", pattern, err)

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
			err = fmt.Errorf("fiber: route registration panicked for %s %s: %v\n%s", method, pattern, rec, stack)
		}
	}()

	r.Add(method, pattern, handler)

	return nil
}

func (d *fiberDriver) routerLogf(format string, args ...any) {
	d.log().Warn().Msgf(format, args...)
}

func wrapHandler(handler http.HandlerFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler(w, r.WithContext(router.WithParams(r.Context(), c.AllParams())))
		})
		serveHTTP(c, h)

		return nil
	}
}

// wrapMiddleware bridges net/http middleware to a fiber handler.
//
// gofiber/adaptor.HTTPMiddleware is unusable here: since fasthttp v1.62 the
// underlying fasthttpadaptor buffers each conversion and applies it with
// Response.SetBody, so every chained handler replaces the response body and
// middleware bytes written before next.ServeHTTP are lost. This bridge keeps
// append semantics: middleware output is appended to the fiber response body
// before continuing the chain with c.Next.
func wrapMiddleware(mw func(http.Handler) http.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var next bool

		nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			next = true
			// Propagate request mutations back to the fiber ctx, mirroring
			// gofiber/adaptor.HTTPMiddleware.
			c.Request().Header.SetMethod(r.Method)
			c.Request().SetRequestURI(r.RequestURI)
			c.Request().SetHost(r.Host)
			for key, val := range r.Header {
				for _, v := range val {
					c.Request().Header.Set(key, v)
				}
			}
		})
		serveHTTP(c, mw(nextHandler))
		if next {
			return c.Next()
		}

		return nil
	}
}

// recorderPool recycles *httptest.ResponseRecorder values across serveHTTP
// calls. A recorder is used only for the duration of one serveHTTP: after the
// handler returns its status/headers/body are copied into the fiber
// response, so the recorder can be reset and reused. The body bytes are copied
// out by fasthttp's AppendBody (which never aliases its argument), so no
// recorder buffer escapes the call and the reset is race-free.
var recorderPool = sync.Pool{
	New: func() any { return httptest.NewRecorder() },
}

// resetRecorder returns rec to a fresh-from-NewRecorder state so it can be
// reused, preserving its HeaderMap and Body allocations. The whole-struct
// assignment is what clears the unexported wroteHeader/result/snapHeader
// fields -- a field-by-field reset cannot reach them, and a stale wroteHeader
// would make the next handler's WriteHeader a silent no-op.
func resetRecorder(rec *httptest.ResponseRecorder) {
	header := rec.HeaderMap
	body := rec.Body
	*rec = httptest.ResponseRecorder{HeaderMap: header, Body: body}

	if header != nil {
		for k := range header {
			delete(header, k)
		}
	}

	if body != nil {
		body.Reset()
	}
}

// serveHTTP runs h against the current fiber request and merges the recorded
// status, headers and body back into the fiber response. The body is appended
// (never replaced) so output from earlier chained handlers is preserved.
func serveHTTP(c *fiber.Ctx, h http.Handler) {
	var r http.Request
	if err := fasthttpadaptor.ConvertRequest(c.Context(), &r, true); err != nil {
		c.Status(fiber.StatusInternalServerError)

		return
	}

	rec := recorderPool.Get().(*httptest.ResponseRecorder)
	defer func() {
		resetRecorder(rec)
		recorderPool.Put(rec)
	}()

	h.ServeHTTP(rec, r.WithContext(c.Context()))

	for k, vv := range rec.Header() {
		for _, v := range vv {
			c.Response().Header.Add(k, v)
		}
	}
	if rec.Code != 0 {
		c.Status(rec.Code)
	}
	if rec.Body.Len() > 0 {
		c.Response().AppendBody(rec.Body.Bytes())
	}
}
