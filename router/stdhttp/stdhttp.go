package stdhttp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/log/noop"
	"github.com/zenta-dev/zever/router"
)

// New creates a router.Router backed by the standard net/http.ServeMux.
//
// AppName is ignored; opts are accepted for registry compatibility.
// Use appends router-wide middleware affecting only later registrations.
func New(opts router.Options) (router.Router, error) {
	return &driver{mux: http.NewServeMux(), routes: make(map[string]bool), logger: opts.Logger}, nil
}

type driver struct {
	mux    *http.ServeMux
	mu     sync.Mutex
	routes map[string]bool
	mws    []func(http.Handler) http.Handler
	logger log.Logger
}

func (d *driver) log() log.Logger {
	if d != nil && d.logger != nil {
		return d.logger
	}

	return noop.New()
}

func (d *driver) Handle(method, pattern string, handler http.HandlerFunc) {
	m, ok := d.validateMethod(method, pattern)
	if !ok {
		return
	}

	p, ok := d.checkPattern(pattern)
	if !ok {
		return
	}

	d.mu.Lock()
	mws := append([]func(http.Handler) http.Handler(nil), d.mws...)
	d.mu.Unlock()

	d.add(m, p, mws, handler)
}

func (d *driver) add(m, p string, mws []func(http.Handler) http.Handler, handler http.HandlerFunc) {
	key := m + " " + p

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.routes[key] {
		d.routerLogf("[router] stdhttp: skipping duplicate route %s %s", m, p)

		return
	}

	if err := d.safeAdd(m+" "+p, buildHandler(p, mws, handler)); err != nil {
		d.routerLogf("[router] stdhttp: failed to register route %s %s: %v", m, p, err)
		delete(d.routes, key)

		return
	}

	d.routes[key] = true
}

func (d *driver) safeAdd(pattern string, h http.Handler) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("stdhttp: route registration panicked for %s: %v", pattern, rec)
		}
	}()

	d.mux.Handle(pattern, h)

	return nil
}

func (d *driver) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	return &stdGroup{d: d, prefix: normalizePrefix(prefix), mws: append([]func(http.Handler) http.Handler(nil), middlewares...)}
}

func (d *driver) Use(middlewares ...func(http.Handler) http.Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mws = append(d.mws, middlewares...)
}

func (d *driver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

type stdGroup struct {
	d      *driver
	prefix string
	mws    []func(http.Handler) http.Handler
}

func (g *stdGroup) Handle(method, pattern string, handler http.HandlerFunc) {
	m, ok := g.d.validateMethod(method, pattern)
	if !ok {
		return
	}

	if strings.TrimSpace(pattern) == "" {
		g.d.routerLogf("[router] stdhttp: skipping route with empty pattern")

		return
	}

	full := joinPath(g.prefix, pattern)

	if _, ok := g.d.checkPattern(full); !ok {
		return
	}

	g.d.mu.Lock()
	mws := append([]func(http.Handler) http.Handler(nil), g.mws...)
	g.d.mu.Unlock()

	g.d.add(m, full, mws, handler)
}

func (g *stdGroup) Group(prefix string, middlewares ...func(http.Handler) http.Handler) router.Group {
	full := joinPath(g.prefix, prefix)
	if stripped := strings.TrimRight(full, "/"); stripped != "" {
		full = stripped
	} else {
		full = "/"
	}

	mws := append(append([]func(http.Handler) http.Handler(nil), g.mws...), middlewares...)

	return &stdGroup{d: g.d, prefix: full, mws: mws}
}

func (g *stdGroup) Use(middlewares ...func(http.Handler) http.Handler) {
	g.d.mu.Lock()
	defer g.d.mu.Unlock()

	g.mws = append(g.mws, middlewares...)
}

func (d *driver) validateMethod(method, pattern string) (string, bool) {
	m := strings.ToUpper(strings.TrimSpace(method))
	if !router.ValidMethod(m) {
		d.routerLogf("[router] stdhttp: skipping route with unsupported method %q (%s)", method, pattern)

		return "", false
	}

	return m, true
}

func (d *driver) checkPattern(pattern string) (string, bool) {
	p := strings.TrimSpace(pattern)
	if p == "" {
		d.routerLogf("[router] stdhttp: skipping route with empty pattern")

		return "", false
	}

	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	if hasBraceColon(p) {
		d.routerLogf("[router] stdhttp: skipping regex-constraint pattern %q (use plain {name} wildcards)", pattern)

		return "", false
	}

	if hasColonSegment(p) {
		d.routerLogf("[router] stdhttp: skipping colon-style pattern %q (use {name} wildcards)", pattern)

		return "", false
	}

	if strings.Contains(p, "<") || strings.Contains(p, ">") {
		d.routerLogf("[router] stdhttp: skipping regex-constraint pattern %q (use plain {name} wildcards)", pattern)

		return "", false
	}

	return p, true
}

func hasColonSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.Contains(seg, ":") {
			return true
		}
	}

	return false
}

func hasBraceColon(p string) bool {
	for i := 0; i < len(p); i++ {
		if p[i] != '{' {
			continue
		}

		j := strings.IndexByte(p[i:], '}')
		if j == -1 {
			break
		}

		if strings.Contains(p[i+1:i+j], ":") {
			return true
		}

		i += j
	}

	return false
}

func paramNames(p string) []string {
	var out []string

	for i := 0; i < len(p); i++ {
		if p[i] != '{' {
			continue
		}

		j := strings.IndexByte(p[i:], '}')
		if j == -1 {
			break
		}

		body := strings.TrimSuffix(p[i+1:i+j], "...")
		if body == "" || body == "$" {
			i += j

			continue
		}

		out = append(out, body)
		i += j
	}

	return out
}

func buildHandler(path string, mws []func(http.Handler) http.Handler, handler http.HandlerFunc) http.Handler {
	names := paramNames(path)

	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(names) == 0 {
			handler(w, r)

			return
		}

		params := make(map[string]string, len(names))
		for _, n := range names {
			params[n] = r.PathValue(n)
		}

		handler(w, r.WithContext(router.WithParams(r.Context(), params)))
	})

	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}

	return h
}

func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}

	return p
}

func joinPath(prefix, pattern string) string {
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(strings.TrimSpace(pattern), "/")
}

func (d *driver) routerLogf(format string, args ...any) {
	d.log().Warn().Msgf(format, args...)
}
