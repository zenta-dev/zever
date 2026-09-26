package stdhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

func newTestRouter(t *testing.T) router.Router {
	t.Helper()

	r, err := New(router.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if r == nil {
		t.Fatal("New() returned nil router")
	}

	return r
}

// TestNewRejectsInvalidOptions covers the fix for New skipping
// opts.Validate() entirely, unlike router/fiber.New: an AppName invalid for
// every adapter (a NUL byte, per router.Options.Validate) previously
// succeeded silently against stdhttp while failing against fiber, breaking
// adapter-swap transparency.
func TestNewRejectsInvalidOptions(t *testing.T) {
	_, err := New(router.Options{AppName: "bad\x00name"})
	if err == nil {
		t.Fatal("New() with an invalid AppName succeeded, want an error")
	}
}

func serve(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func headerMW(key, val string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, val)
			next.ServeHTTP(w, r)
		})
	}
}

func orderMW(name string, order *[]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*order = append(*order, name)
			next.ServeHTTP(w, r)
		})
	}
}

func TestNew_ignoresOptions(t *testing.T) {
	for _, opts := range []router.Options{{}, {AppName: "myapp"}} {
		r, err := New(opts)
		if err != nil {
			t.Fatalf("New(%v) error = %v", opts, err)
		}

		if r == nil {
			t.Fatal("New() returned nil router")
		}
	}
}

func TestHandle_happyPath(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	})

	rec := serve(t, r, "GET", "/hello")
	if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
		t.Fatalf("got %d %q, want 200 %q", rec.Code, rec.Body.String(), "hi")
	}
}

func TestHandle_noLeadingSlash(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "users/list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("list"))
	})

	if rec := serve(t, r, "GET", "/users/list"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestHandle_paramBridging(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		if got := router.Param(req, "id"); got != "42" {
			t.Errorf("Param(id) = %q, want %q", got, "42")
		}

		names := router.ParamNames(req)
		if names["id"] != "42" || len(names) != 1 {
			t.Errorf("ParamNames() = %v, want map[id:42]", names)
		}

		_, _ = w.Write([]byte("ok"))
	})

	if rec := serve(t, r, "GET", "/users/42"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestHandle_multipleParams(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/a/{x}/b/{y}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "x") + "," + router.Param(req, "y")))
	})

	rec := serve(t, r, "GET", "/a/1/b/2")
	if rec.Body.String() != "1,2" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "1,2")
	}
}

func TestHandle_restSubtree(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/files/{rest...}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "rest")))
	})

	rec := serve(t, r, "GET", "/files/a/b/c")
	if rec.Body.String() != "a/b/c" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "a/b/c")
	}
}

func TestHandle_exactMarker(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/exact/{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("exact"))
	})

	if rec := serve(t, r, "GET", "/exact/"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	if rec := serve(t, r, "GET", "/exact/other"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_methodNotAllowed(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/m", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("m"))
	})

	rec := serve(t, r, "POST", "/m")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", rec.Code)
	}

	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q, want it to contain GET", allow)
	}
}

func TestHandle_headOnGet(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/h", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("h"))
	})

	if rec := serve(t, r, "HEAD", "/h"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestHandle_invalidMethodRejected(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("BOGUS", "/nope", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("nope"))
	})

	if rec := serve(t, r, "GET", "/nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_emptyPatternRejected(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})
	r.Handle("GET", "   ", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	if rec := serve(t, r, "GET", "/"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_colonPatternRejected(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/users/:id", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	if rec := serve(t, r, "GET", "/users/1"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_anglePatternsRejected(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/a/<re>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})
	r.Handle("GET", "/b/q>", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	if rec := serve(t, r, "GET", "/a/x"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}

	if rec := serve(t, r, "GET", "/b/q>"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_braceRegexRejected(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/users/{id:[0-9]+}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	if rec := serve(t, r, "GET", "/users/1"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_emptyBracesSkipped(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/{}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	if rec := serve(t, r, "GET", "/zzz"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestHandle_unclosedBraceSkippedRetryWorks(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/u1/{id", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bad"))
	})
	r.Handle("GET", "/u1/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	rec := serve(t, r, "GET", "/u1/42")
	if rec.Body.String() != "42" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "42")
	}
}

func TestHandle_duplicateFirstWins(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/dup", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
	})
	r.Handle("GET", "/dup", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("second"))
	})

	if rec := serve(t, r, "GET", "/dup"); rec.Body.String() != "first" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "first")
	}
}

func TestHandle_conflictSkippedRetryWorks(t *testing.T) {
	r := newTestRouter(t)
	ok := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}
	r.Handle("GET", "/cf/{id}/status/", ok)
	r.Handle("GET", "/cf/0/{action}/", ok)
	r.Handle("GET", "/cf/{id}/other/", ok)

	if rec := serve(t, r, "GET", "/cf/123/status/"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	if rec := serve(t, r, "GET", "/cf/0/run/"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}

	if rec := serve(t, r, "GET", "/cf/123/other/"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestGroup_prefixJoinMiddlewareOrderIsolation(t *testing.T) {
	r := newTestRouter(t)

	var order []string

	r.Handle("GET", "/plain", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plain"))
	})

	g := r.Group("/api", orderMW("g1", &order), orderMW("g2", &order))
	g.Handle("GET", "/users", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("users"))
	})

	rec := serve(t, r, "GET", "/api/users")
	if rec.Body.String() != "users" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "users")
	}

	if len(order) != 2 || order[0] != "g1" || order[1] != "g2" {
		t.Fatalf("order = %v, want [g1 g2]", order)
	}

	order = nil

	if rec := serve(t, r, "GET", "/plain"); rec.Body.String() != "plain" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "plain")
	}

	if len(order) != 0 {
		t.Fatalf("order = %v, want empty (group mw must not leak)", order)
	}
}

func TestGroup_nested(t *testing.T) {
	r := newTestRouter(t)
	v1 := r.Group("/v1")
	users := v1.Group("/users", headerMW("X-Nested", "yes"))
	users.Handle("GET", "/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	rec := serve(t, r, "GET", "/v1/users/7")
	if rec.Body.String() != "7" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "7")
	}

	if rec.Header().Get("X-Nested") != "yes" {
		t.Fatalf("X-Nested = %q, want yes", rec.Header().Get("X-Nested"))
	}
}

func TestGroup_prefixEdgeCases(t *testing.T) {
	r := newTestRouter(t)
	ok := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}

	r.Group("").Handle("GET", "/root", ok)
	r.Group("api").Handle("GET", "/list", ok)
	r.Group("/t/").Handle("GET", "/x", ok)

	for _, target := range []string{"/root", "/api/list", "/t/x"} {
		if rec := serve(t, r, "GET", target); rec.Body.String() != "ok" {
			t.Fatalf("GET %s: got %q, want ok", target, rec.Body.String())
		}
	}
}

func TestGroup_rootNestedEmpty(t *testing.T) {
	r := newTestRouter(t)
	sub := r.Group("/").Group("")
	sub.Handle("GET", "/deep", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("deep"))
	})

	if rec := serve(t, r, "GET", "/deep"); rec.Body.String() != "deep" {
		t.Fatalf("got %q, want deep", rec.Body.String())
	}
}

func TestGroup_emptyPatternRejected(t *testing.T) {
	r := newTestRouter(t)
	g := r.Group("/g")
	g.Handle("GET", "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})
	g.Handle("BOGUS", "/y", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})
	g.Handle("GET", "/z/:bad", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	})

	for _, target := range []string{"/g", "/g/y", "/g/z/1"} {
		if rec := serve(t, r, "GET", target); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s: got %d, want 404", target, rec.Code)
		}
	}
}

func TestUse_appliesToLaterRoutesOnly(t *testing.T) {
	r := newTestRouter(t)
	ok := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}

	r.Handle("GET", "/before", ok)
	r.Use(headerMW("X-Late", "1"))
	r.Handle("GET", "/after", ok)

	if rec := serve(t, r, "GET", "/before"); rec.Header().Get("X-Late") != "" {
		t.Fatalf("X-Late = %q, want empty for pre-Use route", rec.Header().Get("X-Late"))
	}

	if rec := serve(t, r, "GET", "/after"); rec.Header().Get("X-Late") != "1" {
		t.Fatalf("X-Late = %q, want 1 for post-Use route", rec.Header().Get("X-Late"))
	}
}

func TestGroupUse_appliesToLaterGroupRoutes(t *testing.T) {
	r := newTestRouter(t)
	ok := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}

	g := r.Group("/gu")
	g.Handle("GET", "/a", ok)
	g.Use(headerMW("X-G", "1"))
	g.Handle("GET", "/b", ok)

	if rec := serve(t, r, "GET", "/gu/a"); rec.Header().Get("X-G") != "" {
		t.Fatalf("X-G = %q, want empty", rec.Header().Get("X-G"))
	}

	if rec := serve(t, r, "GET", "/gu/b"); rec.Header().Get("X-G") != "1" {
		t.Fatalf("X-G = %q, want 1", rec.Header().Get("X-G"))
	}
}

func TestServeHTTP_concurrent(t *testing.T) {
	r := newTestRouter(t)
	r.Handle("GET", "/conc/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			rec := serve(t, r, "GET", "/conc/9")
			if rec.Body.String() != "9" {
				t.Errorf("got %q, want 9", rec.Body.String())
			}
		}()
	}

	wg.Wait()
}
