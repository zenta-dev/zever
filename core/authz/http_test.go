package authz_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/authz"
)

func TestBearerTokenTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"standard", "Bearer abc", "abc"},
		{"lowercase scheme", "bearer abc", "abc"},
		{"upper scheme", "BEARER abc", "abc"},
		{"surrounding spaces", "  Bearer abc  ", "abc"},
		{"token padded", "Bearer   abc   ", "abc"},
		{"non-Bearer", "Basic abc", ""},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"bare Bearer no token", "Bearer ", ""},
		{"inner space", "Bearer a  b", ""},
		{"inner tab", "Bearer a\tb", ""},
		{"no scheme", "abc", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			if got := authz.BearerToken(r); got != tc.want {
				t.Fatalf("BearerToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBearerTokenNilRequest(t *testing.T) {
	t.Parallel()

	if got := authz.BearerToken(nil); got != "" {
		t.Fatalf("BearerToken(nil) = %q, want empty", got)
	}
}

func TestMiddleware401AndHandlerNotCalled(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := authz.Middleware(a, p, authz.Policy{AuthRequired: true}, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran on auth failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="api"` {
		t.Fatalf("WWW-Authenticate = %q, want bare realm (no credentials presented)", got)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["error"] == "" {
		t.Fatal("body.error empty")
	}
}

func TestMiddleware401BadTokenChallenge(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := authz.Middleware(a, p, authz.Policy{AuthRequired: true}, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran on auth failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); !strings.Contains(got, `Bearer realm="api"`) || !strings.Contains(got, `error="invalid_token"`) {
		t.Fatalf("WWW-Authenticate = %q, want Bearer realm + invalid_token", got)
	}
}

func TestMiddlewareDuplicateAuthHeaderRejected(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := authz.Middleware(a, p, authz.Policy{AuthRequired: true}, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Add("Authorization", "Bearer good")
	r.Header.Add("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran with duplicate Authorization headers")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401 (fail closed)", w.Code)
	}
}

func TestMiddleware403(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: false}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	h := authz.Middleware(a, p, pol, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran on denial")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", w.Code)
	}
}

func TestMiddlewareSuccessClaimsRoundtrip(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "user-7"}}
	p := &fakeChecker{allow: true}
	var gotSubject string
	var ok bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var c auth.Claims
		c, ok = authz.ClaimsFromContext(r.Context())
		gotSubject = c.Subject
	})
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	h := authz.Middleware(a, p, pol, func(*http.Request) string { return "res-1" })(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !ok || gotSubject != "user-7" {
		t.Fatalf("claims roundtrip = (%q, %v), want (user-7, true)", gotSubject, ok)
	}
	if !p.called || p.gotResource.ID != "res-1" {
		t.Fatalf("resourceID fn not wired: called=%v res=%+v", p.called, p.gotResource)
	}
}

func TestMiddlewareNilResourceIDFn(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	h := authz.Middleware(a, p, pol, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
}
