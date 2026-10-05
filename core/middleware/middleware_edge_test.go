package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareEdge_CORSHeadersAllowed(t *testing.T) {
	t.Parallel()

	allowed := map[string]bool{"content-type": true, "authorization": true}

	tests := []struct {
		name      string
		requested string
		want      bool
	}{
		{name: "empty passes", requested: "", want: true},
		{name: "whitespace passes", requested: "   ", want: true},
		{name: "single allowed", requested: "Content-Type", want: true},
		{name: "case insensitive", requested: "AUTHORIZATION", want: true},
		{name: "multiple allowed", requested: "Content-Type, Authorization", want: true},
		{name: "unknown denied", requested: "X-Evil", want: false},
		{name: "one unknown denied", requested: "Content-Type, X-Evil", want: false},
		{name: "blank entries skipped", requested: "Content-Type, , ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := corsHeadersAllowed(tt.requested, allowed); got != tt.want {
				t.Fatalf("corsHeadersAllowed(%q) = %v, want %v", tt.requested, got, tt.want)
			}
		})
	}
}

func TestMiddlewareEdge_AddVary(t *testing.T) {
	t.Parallel()

	t.Run("appends new names", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		addVary(rec, "Origin", "Accept-Encoding")

		if got, want := rec.Header().Get("Vary"), "Origin, Accept-Encoding"; got != want {
			t.Fatalf("Vary = %q, want %q", got, want)
		}
	})

	t.Run("dedupes case-insensitively", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		addVary(rec, "Origin")
		addVary(rec, "origin")

		if got, want := rec.Header().Get("Vary"), "Origin"; got != want {
			t.Fatalf("Vary = %q, want %q", got, want)
		}
	})

	t.Run("preserves existing entries", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		rec.Header().Set("Vary", "Accept")

		addVary(rec, "Origin")

		if got, want := rec.Header().Get("Vary"), "Accept, Origin"; got != want {
			t.Fatalf("Vary = %q, want %q", got, want)
		}
	})
}

func TestMiddlewareEdge_NormalizeMethods(t *testing.T) {
	t.Parallel()

	got := normalizeMethods([]string{" get ", "Post", "PUT"})
	want := []string{"GET", "POST", "PUT"}

	if len(got) != len(want) {
		t.Fatalf("normalizeMethods() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeMethods()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if empty := normalizeMethods(nil); len(empty) != 0 {
		t.Fatalf("normalizeMethods(nil) = %v, want empty", empty)
	}
}

func TestMiddlewareEdge_CORSPreflightNoRequestMethod(t *testing.T) {
	t.Parallel()

	nextRan := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextRan = true
		w.WriteHeader(http.StatusOK)
	})

	handler := CORS(testCORSOptions())(next)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !nextRan {
		t.Fatal("OPTIONS without Access-Control-Request-Method must be treated as a simple request")
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Allow-Origin = %q, want https://example.com", got)
	}
}

func TestMiddlewareEdge_CORSMaxAgeOmitted(t *testing.T) {
	t.Parallel()

	opts := testCORSOptions()
	opts.MaxAge = 0

	handler := CORS(opts)(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Max-Age"); got != "" {
		t.Fatalf("Access-Control-Max-Age = %q, want omitted when MaxAge is unset", got)
	}
}

func TestMiddlewareEdge_CORSMaxAgeSubSecondTruncatesToZero(t *testing.T) {
	t.Parallel()

	// Observed current behavior: a positive sub-second MaxAge passes the
	// `MaxAge > 0` guard and truncates to "0" on the wire. This diverges from
	// the Options.MaxAge doc comment ("anything below one second omits the
	// header"); captured here so the edge is not silently untested.
	opts := testCORSOptions()
	opts.MaxAge = 500 * time.Millisecond

	handler := CORS(opts)(okHandler())

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Max-Age"); got != "0" {
		t.Fatalf("Access-Control-Max-Age = %q, want 0 (truncated)", got)
	}
}

func TestMiddlewareEdge_StatusRecorderWriteThenWriteHeader(t *testing.T) {
	t.Parallel()

	inner := &bareWriter{header: make(http.Header)}
	rec := newStatusRecorder(inner)

	if _, err := rec.Write([]byte("body")); err != nil {
		t.Fatalf("Write error = %v", err)
	}

	rec.WriteHeader(http.StatusInternalServerError)

	if rec.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (implicit status preserved after Write)", rec.status)
	}
}
