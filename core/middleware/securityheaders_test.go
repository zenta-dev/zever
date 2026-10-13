package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveHeaders(t *testing.T, opts SecurityHeadersOptions, handler http.Handler) http.Header {
	t.Helper()

	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/", nil)
	SecurityHeaders(opts)(handler).ServeHTTP(rec, req)

	return rec.Result().Header
}

func TestSecurityHeadersDefaults(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{}, nil)

	if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := header.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy = %q, want strict-origin-when-cross-origin", got)
	}
	if got := header.Get("Permissions-Policy"); got != "camera=(), microphone=(), geolocation=()" {
		t.Fatalf("Permissions-Policy = %q, want locked-down default", got)
	}
	if got := header.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if got := header.Get("Content-Security-Policy"); got != "" {
		t.Fatalf("Content-Security-Policy = %q, want unset by default", got)
	}
	// HSTS on plaintext is a spec violation: zero HSTSMaxAge (the
	// default, used when TLS is off) must omit the header.
	if got := header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want unset without HSTSMaxAge", got)
	}
}

func TestSecurityHeadersHSTS(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{HSTSMaxAge: DefaultHSTSMaxAge}, nil)

	got := header.Get("Strict-Transport-Security")
	if !strings.HasPrefix(got, "max-age=31536000") {
		t.Fatalf("Strict-Transport-Security = %q, want max-age=31536000 prefix", got)
	}
	if !strings.Contains(got, "includeSubDomains") {
		t.Fatalf("Strict-Transport-Security = %q, want includeSubDomains", got)
	}
}

func TestSecurityHeadersConfigurable(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{
		FrameOptions:          "DENY",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "camera=(self)",
	}, nil)

	if got := header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := header.Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Fatalf("Content-Security-Policy = %q, want configured policy", got)
	}
	if got := header.Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := header.Get("Permissions-Policy"); got != "camera=(self)" {
		t.Fatalf("Permissions-Policy = %q, want configured policy", got)
	}
}

func TestSecurityHeadersCSPOmitsFrameOptions(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{
		ContentSecurityPolicy: "default-src 'self'; frame-ancestors 'none'",
	}, nil)

	if got := header.Get("Content-Security-Policy"); got != "default-src 'self'; frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q, want configured policy", got)
	}
	if got := header.Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options = %q, want omitted when CSP owns framing", got)
	}
}

func TestSecurityHeadersPreserveHandlerResponse(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "kept")
		w.WriteHeader(http.StatusTeapot)
	}))

	if got := header.Get("X-Custom"); got != "kept" {
		t.Fatalf("X-Custom = %q, want kept", got)
	}
	if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff alongside handler headers", got)
	}
}

func TestSecurityHeadersInvalidFailsClosed(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/", nil)
	SecurityHeaders(SecurityHeadersOptions{HSTSMaxAge: -1})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler must not run with invalid options")
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "" {
		t.Fatalf("X-Content-Type-Options = %q, want unset on the fail-closed path", got)
	}
}
