// Package routertest provides the conformance kit third-party router adapters run to prove backend parity.
package routertest

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

// Conformance verifies factory-built backends implement the
// router.Router contract: open/register round-trip, direct route
// serving, path params, group prefixes, Use middleware, and 404 for
// unknown paths. Each subtest takes a fresh instance from factory so
// cases stay isolated. Tests serve over httptest (no external
// network) and never call time.Sleep.
//
// Method-not-found codes stay adapter-owned: mux backends answer 405
// where others answer 404, so the kit only pins 200 bodies and the
// unknown-path 404 both adapters share.
func Conformance(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("ServeRoute", func(t *testing.T) { conformanceServeRoute(t, factory) })
	t.Run("PathParam", func(t *testing.T) { conformancePathParam(t, factory) })
	t.Run("Group", func(t *testing.T) { conformanceGroup(t, factory) })
	t.Run("Use", func(t *testing.T) { conformanceUse(t, factory) })
	t.Run("NotFound", func(t *testing.T) { conformanceNotFound(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := router.Open(router.Adapter("conformance-missing-adapter"), router.Options{}); !errors.Is(err, router.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := router.Adapter("conformance-probe-router")

	if err := router.Register(probe, nil); !errors.Is(err, router.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(router.Options) (router.Router, error) {
		return nil, errors.New("routertest: probe factory must not run")
	}

	_ = router.Register(probe, stub)

	if err := router.Register(probe, stub); !errors.Is(err, router.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

// serve issues req against r and returns the recorded response.
func serve(t *testing.T, r router.Router, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	return rec
}

// bodyOf drains rec into a string or fails the test.
func bodyOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	data, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}

	return string(data)
}

func conformanceServeRoute(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	r := factory(t)
	r.Handle(http.MethodGet, "/kit/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})

	rec := serve(t, r, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit/hello", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /kit/hello code = %d, want 200", rec.Code)
	}

	if got := bodyOf(t, rec); got != "hello" {
		t.Errorf("GET /kit/hello body = %q, want hello", got)
	}
}

func conformancePathParam(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	r := factory(t)
	r.Handle(http.MethodGet, "/kit/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		if got := router.Param(req, "id"); got != "42" {
			http.Error(w, "bad param", http.StatusInternalServerError)

			return
		}

		_, _ = w.Write([]byte("42"))
	})

	rec := serve(t, r, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit/users/42", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /kit/users/42 code = %d, want 200", rec.Code)
	}

	if got := bodyOf(t, rec); got != "42" {
		t.Errorf("GET /kit/users/42 body = %q, want 42", got)
	}
}

func conformanceGroup(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	r := factory(t)
	g := r.Group("/kit/api")
	g.Handle(http.MethodGet, "/items", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("items"))
	})

	rec := serve(t, r, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit/api/items", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /kit/api/items code = %d, want 200", rec.Code)
	}

	if got := bodyOf(t, rec); got != "items" {
		t.Errorf("GET /kit/api/items body = %q, want items", got)
	}
}

func conformanceUse(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	r := factory(t)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Kit-Middleware", "hit")
			next.ServeHTTP(w, req)
		})
	})
	r.Handle(http.MethodGet, "/kit/mw", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	rec := serve(t, r, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit/mw", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /kit/mw code = %d, want 200", rec.Code)
	}

	if got := rec.Header().Get("X-Kit-Middleware"); got != "hit" {
		t.Errorf("X-Kit-Middleware = %q, want hit", got)
	}
}

func conformanceNotFound(t *testing.T, factory func(t *testing.T) router.Router) {
	t.Helper()

	r := factory(t)
	r.Handle(http.MethodGet, "/kit/exists", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	rec := serve(t, r, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit/does-not-exist", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown code = %d, want 404", rec.Code)
	}
}
