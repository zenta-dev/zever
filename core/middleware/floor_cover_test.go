package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestTimeout_ImplicitOKDefaultsTo200 pins that a handler writing a body
// without an explicit WriteHeader still gets a 200 through Timeout: the
// buffered copy path and timeoutWriter.Write both default an unset code to
// StatusOK instead of emitting a headerless 0 status.
func TestTimeout_ImplicitOKDefaultsTo200(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit"))
	})

	wrapped := Timeout(time.Second)(handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "implicit" {
		t.Fatalf("Body = %q, want implicit", rec.Body.String())
	}
}

// TestTimeout_SilentHandlerDefaultsTo200 pins that a handler returning
// without any write still gets a 200 through Timeout: the buffered copy
// path defaults the unset code instead of emitting a headerless 0 status.
func TestTimeout_SilentHandlerDefaultsTo200(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Marker", "silent")
	})

	wrapped := Timeout(time.Second)(handler)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Marker"); got != "silent" {
		t.Fatalf("X-Marker = %q, want silent", got)
	}
}

// TestRecover_TimeoutGoroutinePanicResponds500 pins the #605/#622 path
// end to end: a handler panicking on Timeout's worker goroutine is
// re-raised on the request goroutine, caught by an outer Recover, answered
// with the fixed 500 envelope, and logged with credentials scrubbed.
func TestRecover_TimeoutGoroutinePanicResponds500(t *testing.T) {
	t.Parallel()

	logger := &panicLogger{}

	secret := errors.New("dial postgres://app:hunter2@db:5432/app: refused")
	handler := Recover(logger)(Timeout(time.Second)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(secret)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"error\":\"internal error\"}\n" {
		t.Fatalf("body = %q, want exact fixed-shape 500 JSON", got)
	}

	errs := logger.errorCalls()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error log, got %d", len(logger.events))
	}
	panStr, ok := errs[0].fields["panic"].(string)
	if !ok {
		t.Fatalf("log fields missing panic value, got %v", errs[0].fields)
	}
	if strings.Contains(panStr, "hunter2") {
		t.Fatalf("log leaked panic credentials: %q", panStr)
	}
	if !strings.Contains(panStr, redacted) {
		t.Fatalf("log missing redaction marker: %q", panStr)
	}
}

// TestRedactPanicValueTruncatesOnRuneBoundary pins that truncation backs
// off a mid-rune cut so the logged value stays valid UTF-8: the leading
// byte shifts the 1024-byte cut onto a continuation byte.
func TestRedactPanicValueTruncatesOnRuneBoundary(t *testing.T) {
	t.Parallel()

	got := redactPanicValue("x" + strings.Repeat("\u00e9", 600))

	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("missing truncation marker: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated value is not valid UTF-8: %q", got)
	}
	if len(got) > maxPanicValueLen+len("...(truncated)") {
		t.Fatalf("len = %d, want <= %d", len(got), maxPanicValueLen+len("...(truncated)"))
	}
}

// TestCORS_validateRejectsShapes pins each fail-open rejection so every
// Validate branch stays covered: empty methods, blank origin, blank
// method, negative MaxAge.
func TestCORS_validateRejectsShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Options)
	}{
		{"empty methods", func(o *Options) { o.AllowedMethods = nil }},
		{"blank origin", func(o *Options) { o.AllowedOrigins = []string{"   "} }},
		{"blank method", func(o *Options) { o.AllowedMethods = []string{"  "} }},
		{"negative max age", func(o *Options) { o.MaxAge = -time.Second }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := testCORSOptions()
			tc.mutate(&opts)
			if err := opts.Validate(); err == nil {
				t.Fatalf("Validate = nil, want error for %s", tc.name)
			}
		})
	}
}

// TestRightmostUntrusted_BlankHeader pins that a whitespace-only
// X-Forwarded-For yields no client instead of a bogus empty match.
func TestRightmostUntrusted_BlankHeader(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"", "   "} {
		client, ok := rightmostUntrusted(header, nil)
		if ok || client != "" {
			t.Fatalf("rightmostUntrusted(%q) = (%q, %v), want (\"\", false)", header, client, ok)
		}
	}
}

// TestSecurityHeadersHSTSPreload pins the preload directive on the wire
// alongside the default includeSubDomains.
func TestSecurityHeadersHSTSPreload(t *testing.T) {
	t.Parallel()

	header := serveHeaders(t, SecurityHeadersOptions{HSTSMaxAge: DefaultHSTSMaxAge, HSTSPreload: true}, nil)

	got := header.Get("Strict-Transport-Security")
	if !strings.HasPrefix(got, "max-age=31536000") {
		t.Fatalf("Strict-Transport-Security = %q, want max-age=31536000 prefix", got)
	}
	if !strings.Contains(got, "; preload") {
		t.Fatalf("Strict-Transport-Security = %q, want preload directive", got)
	}
}
