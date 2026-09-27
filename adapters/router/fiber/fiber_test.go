package fiber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

func newRouter(t *testing.T, opts router.Options) router.Router {
	t.Helper()

	r, err := New(opts)
	if err != nil {
		t.Fatalf("New(%+v): %v", opts, err)
	}

	return r
}

func serve(t *testing.T, r router.Router, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	return resp
}

type serveCase struct {
	path string
	want int
	body string
}

func serveCases(t *testing.T, r router.Router, cases []serveCase) {
	t.Helper()

	for _, tc := range cases {
		resp := serve(t, r, tc.path)
		if resp.Code != tc.want {
			t.Fatalf("expected %s %d, got %d", tc.path, tc.want, resp.Code)
		}

		if tc.want == http.StatusOK && resp.Body.String() != tc.body {
			t.Fatalf("expected %s body %q, got %q", tc.path, tc.body, resp.Body.String())
		}
	}
}

func captureLog(t *testing.T) *captureLogger {
	t.Helper()

	return &captureLogger{}
}

func TestNew_defaultAppName(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("world"))
	})

	resp := serve(t, r, "/hello")
	if resp.Code != http.StatusOK || resp.Body.String() != "world" {
		t.Fatalf("expected 200 world, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestNew_customAppName(t *testing.T) {
	r := newRouter(t, router.Options{AppName: "test-app"})
	r.Handle("GET", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("world"))
	})

	resp := serve(t, r, "/hello")
	if resp.Code != http.StatusOK || resp.Body.String() != "world" {
		t.Fatalf("expected 200 world, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestNew_invalidOptions(t *testing.T) {
	for _, opts := range []router.Options{
		{AppName: strings.Repeat("a", 65)},
		{AppName: "bad\x00name"},
	} {
		if _, err := New(opts); err == nil {
			t.Fatalf("expected error for opts %+v", opts)
		}
	}
}

func TestFiberHandleAndServe(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("world"))
	})

	resp := serve(t, r, "/hello")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	if got := resp.Body.String(); got != "world" {
		t.Fatalf("expected 'world', got %q", got)
	}
}

func TestFiberLowercaseMethod(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("get", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("world"))
	})

	resp := serve(t, r, "/hello")
	if resp.Code != http.StatusOK || resp.Body.String() != "world" {
		t.Fatalf("expected 200 world, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberParamsCurlyPattern(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	resp := serve(t, r, "/users/42")
	if resp.Code != http.StatusOK || resp.Body.String() != "42" {
		t.Fatalf("expected 200 42, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberParamsColonPattern(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/orders/:id", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	resp := serve(t, r, "/orders/7")
	if resp.Code != http.StatusOK || resp.Body.String() != "7" {
		t.Fatalf("expected 200 7, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberCaseDistinctRoutes(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/Users/:id", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("upper:" + router.Param(req, "id")))
	})
	r.Handle("GET", "/users/:id", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("lower:" + router.Param(req, "id")))
	})

	resp := serve(t, r, "/Users/42")
	if resp.Code != http.StatusOK || resp.Body.String() != "upper:42" {
		t.Fatalf("expected upper handler on /Users/42, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/users/42")
	if resp.Code != http.StatusOK || resp.Body.String() != "lower:42" {
		t.Fatalf("expected lower handler on /users/42, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberUnknownMethodSkipped(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})
	r.Handle("BREW", "/brew", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	})

	if !strings.Contains(buf.String(), "unsupported method") {
		t.Fatalf("expected unsupported-method log, got %q", buf.String())
	}

	resp := serve(t, r, "/brew")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unregistered method, got %d", resp.Code)
	}
}

func TestFiberMalformedPatternSkipped(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})
	r.Handle("GET", "/items/:id<", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	})

	if !strings.Contains(buf.String(), "malformed") {
		t.Fatalf("expected malformed-pattern log, got %q", buf.String())
	}

	resp := serve(t, r, "/items/42")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for skipped malformed route, got %d", resp.Code)
	}
}

func TestFiberEmptyPatternSkipped(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	})

	resp := serve(t, r, "/")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when only route was empty, got %d", resp.Code)
	}
}

func TestFiberGroupMiddleware(t *testing.T) {
	r := newRouter(t, router.Options{})

	mw := func(label string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				_, _ = w.Write([]byte("mw" + label + ":"))
				next.ServeHTTP(w, req)
			})
		}
	}

	g := r.Group("/api", mw("a"), mw("b"))
	g.Handle("GET", "/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("handler"))
	})

	resp := serve(t, r, "/api/ping")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	if got := resp.Body.String(); got != "mwa:mwb:handler" {
		t.Fatalf("expected middleware order 'mwa:mwb:handler', got %q", got)
	}
}

func TestFiberUseRouterMiddleware(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte("global:"))
			next.ServeHTTP(w, req)
		})
	})
	r.Handle("GET", "/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("handler"))
	})

	resp := serve(t, r, "/ping")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	if got := resp.Body.String(); got != "global:handler" {
		t.Fatalf("expected 'global:handler', got %q", got)
	}
}

func TestFiberGroupUseMiddleware(t *testing.T) {
	r := newRouter(t, router.Options{})
	g := r.Group("/api")
	g.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte("gmw:"))
			next.ServeHTTP(w, req)
		})
	})
	g.Handle("GET", "/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("handler"))
	})
	r.Handle("GET", "/plain", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plain"))
	})

	resp := serve(t, r, "/api/ping")
	if got := resp.Body.String(); got != "gmw:handler" {
		t.Fatalf("expected 'gmw:handler', got %q", got)
	}

	resp = serve(t, r, "/plain")
	if got := resp.Body.String(); got != "plain" {
		t.Fatalf("expected group middleware not to leak, got %q", got)
	}
}

func TestFiberNativeRegexWrapperMatches(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/items/:num<regex(^[0-9]+$)>", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "num")))
	})

	resp := serve(t, r, "/items/123")
	if resp.Code != http.StatusOK || resp.Body.String() != "123" {
		t.Fatalf("want 200 123, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/items/abc")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-matching, got %d", resp.Code)
	}
}

func TestFiberDuplicateRouteNotRegisteredTwice(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})
	r.Handle("GET", "/dup", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
	})
	r.Handle("GET", "/dup", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("second"))
	})

	if !strings.Contains(buf.String(), "duplicate") {
		t.Fatalf("expected duplicate route to be logged, got %q", buf.String())
	}

	resp := serve(t, r, "/dup")
	if got := resp.Body.String(); got != "first" {
		t.Fatalf("expected only first handler to run, got %q", got)
	}
}

func TestFiberSameRelativeRouteInDifferentGroups(t *testing.T) {
	r := newRouter(t, router.Options{})

	g1 := r.Group("/v1")
	g1.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v1"))
	})

	g2 := r.Group("/v2")
	g2.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v2"))
	})

	resp := serve(t, r, "/v1/health")
	if resp.Code != http.StatusOK || resp.Body.String() != "v1" {
		t.Fatalf("expected v1 handler on /v1/health, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/v2/health")
	if resp.Code != http.StatusOK || resp.Body.String() != "v2" {
		t.Fatalf("expected v2 handler on /v2/health, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberNestedGroupSameRelativeRoute(t *testing.T) {
	r := newRouter(t, router.Options{})

	g := r.Group("/api")
	v1 := g.Group("/v1")
	v1.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("nested"))
	})

	other := r.Group("/v2")
	other.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("other"))
	})

	resp := serve(t, r, "/api/v1/health")
	if resp.Code != http.StatusOK || resp.Body.String() != "nested" {
		t.Fatalf("expected nested handler on /api/v1/health, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/v2/health")
	if resp.Code != http.StatusOK || resp.Body.String() != "other" {
		t.Fatalf("expected other handler on /v2/health, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberGroupColonPrefix(t *testing.T) {
	r := newRouter(t, router.Options{})

	g := r.Group("/orgs/:org")
	g.Handle("GET", "/items/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "org") + ":" + router.Param(req, "id")))
	})

	resp := serve(t, r, "/orgs/acme/items/9")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	if got := resp.Body.String(); got != "acme:9" {
		t.Fatalf("expected 'acme:9', got %q", got)
	}
}

func TestFiberGroupColonPrefixDedup(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})

	g := r.Group("/orgs/:org")
	g.Handle("GET", "/items/:id", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
	})
	g.Handle("GET", "/items/:id", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("second"))
	})

	if !strings.Contains(buf.String(), "duplicate") {
		t.Fatalf("expected duplicate route to be logged, got %q", buf.String())
	}

	resp := serve(t, r, "/orgs/acme/items/9")
	if got := resp.Body.String(); got != "first" {
		t.Fatalf("expected only first handler to run, got %q", got)
	}
}

func TestFiberGroupMalformedHandleSkipped(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})

	g := r.Group("/api")
	g.Handle("GET", "/items/:id<", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	})

	if !strings.Contains(buf.String(), "malformed") {
		t.Fatalf("expected malformed-pattern log, got %q", buf.String())
	}

	resp := serve(t, r, "/api/items/1")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for skipped group route, got %d", resp.Code)
	}
}

func TestFiberGroupMalformedPrefixFallback(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})

	g := r.Group("/bad/:id<")
	if g == nil {
		t.Fatal("expected non-nil group for malformed prefix")
	}

	g.Handle("GET", "/x", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	})

	if !strings.Contains(buf.String(), "malformed") {
		t.Fatalf("expected malformed-pattern log, got %q", buf.String())
	}

	nested := r.Group("/api").Group("/bad/:id<")
	if nested == nil {
		t.Fatal("expected non-nil nested group for malformed prefix")
	}
}

func TestFiberChiStyleRegexRouteMatches(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/items/{num:[0-9]+}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "num")))
	})

	serveCases(t, r, []serveCase{
		{"/items/123", http.StatusOK, "123"},
		{"/items/0", http.StatusOK, "0"},
		{"/items/abc", http.StatusNotFound, ""},
		{"/items/123abc", http.StatusNotFound, ""},
	})
}

func TestFiberChiStyleRegexBraces(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/yrs/{num:[0-9]{4}}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "num")))
	})

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/yrs/1234", http.StatusOK},
		{"/yrs/12", http.StatusNotFound},
		{"/yrs/12345", http.StatusNotFound},
		{"/yrs/abcd", http.StatusNotFound},
	} {
		resp := serve(t, r, tc.path)
		if resp.Code != tc.want {
			t.Fatalf("expected %s %d, got %d", tc.path, tc.want, resp.Code)
		}
	}
}

func TestFiberColonRegexConstraintEnforced(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/items/:id<[0-9]+>", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	serveCases(t, r, []serveCase{
		{"/items/123", http.StatusOK, "123"},
		{"/items/0", http.StatusOK, "0"},
		{"/items/abc", http.StatusNotFound, ""},
		{"/items/12a", http.StatusNotFound, ""},
	})
}

func TestFiberColonBraceRegexDedup(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})
	r.Handle("GET", "/items/{id:[0-9]+}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("brace:" + router.Param(req, "id")))
	})
	r.Handle("GET", "/items/:id<[0-9]+>", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("colon:" + router.Param(req, "id")))
	})

	if !strings.Contains(buf.String(), "duplicate") {
		t.Fatalf("expected duplicate route to be logged, got %q", buf.String())
	}

	resp := serve(t, r, "/items/42")
	if resp.Code != http.StatusOK || resp.Body.String() != "brace:42" {
		t.Fatalf("expected brace handler on /items/42, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/items/abc")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on /items/abc, got %d", resp.Code)
	}
}

func TestFiberNestedGroupMiddleware(t *testing.T) {
	r := newRouter(t, router.Options{})

	g := r.Group("/api")
	v1 := g.Group("/v1", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = w.Write([]byte("nested-mw:"))
			next.ServeHTTP(w, req)
		})
	})
	v1.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	resp := serve(t, r, "/api/v1/health")
	if resp.Code != http.StatusOK || resp.Body.String() != "nested-mw:ok" {
		t.Fatalf("expected nested middleware to run, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberGroupFailedAddNotMarked(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})

	g := r.Group("/g")

	var brokenBuilder strings.Builder
	brokenBuilder.WriteString("/deep")
	for i := 0; i < 31; i++ {
		fmt.Fprintf(&brokenBuilder, "/{p%d}", i)
	}
	broken := brokenBuilder.String()

	never := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never"))
	}
	g.Handle("GET", broken, never)
	g.Handle("GET", "/deep/{first}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "first")))
	})

	if !strings.Contains(buf.String(), "failed to register") {
		t.Fatalf("expected failed registration log, got %q", buf.String())
	}

	if strings.Contains(buf.String(), "duplicate") {
		t.Fatalf("failed group Add must not pre-mark dedup, got %q", buf.String())
	}

	resp := serve(t, r, "/g/deep/x")
	if resp.Code != http.StatusOK || resp.Body.String() != "x" {
		t.Fatalf("expected corrected group route on /g/deep/x, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberGroupColonRegexPrefix(t *testing.T) {
	r := newRouter(t, router.Options{})

	g := r.Group("/orgs/:org<[a-z]+>")
	g.Handle("GET", "/items/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "org") + ":" + router.Param(req, "id")))
	})

	resp := serve(t, r, "/orgs/acme/items/9")
	if resp.Code != http.StatusOK || resp.Body.String() != "acme:9" {
		t.Fatalf("expected acme:9 on /orgs/acme/items/9, got %d %q", resp.Code, resp.Body.String())
	}

	resp = serve(t, r, "/orgs/123/items/9")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on /orgs/123/items/9, got %d", resp.Code)
	}
}

func TestFiberFailedAddNotMarkedAndRetryWorks(t *testing.T) {
	buf := captureLog(t)
	r := newRouter(t, router.Options{Logger: buf})

	var brokenBuilder strings.Builder
	brokenBuilder.WriteString("/deep")
	for i := 0; i < 31; i++ {
		fmt.Fprintf(&brokenBuilder, "/{p%d}", i)
	}
	broken := brokenBuilder.String()

	serveLabel := func(label string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(label))
		}
	}
	r.Handle("GET", broken, serveLabel("never"))
	r.Handle("GET", broken, serveLabel("never2"))
	r.Handle("GET", "/deep/{first}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "first")))
	})

	if !strings.Contains(buf.String(), "failed to register") {
		t.Fatalf("expected failed registration log, got %q", buf.String())
	}

	if strings.Contains(buf.String(), "duplicate") {
		t.Fatalf("failed Add must not pre-mark dedup, got %q", buf.String())
	}

	resp := serve(t, r, "/deep/x")
	if resp.Code != http.StatusOK || resp.Body.String() != "x" {
		t.Fatalf("expected corrected route on /deep/x, got %d %q", resp.Code, resp.Body.String())
	}
}

func TestFiberPanicReturns500(t *testing.T) {
	r := newRouter(t, router.Options{})
	r.Handle("GET", "/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	resp := serve(t, r, "/panic")
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}
