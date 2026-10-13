package api_test

import (
	"net/http"
	"testing"
)

// TestUnauthenticatedNotesRejected is the smallest useful app test: a fresh
// per-test handler (own sqlite file, own container, closed in Cleanup) and
// one in-process request asserting the auth boundary. No network, no clock,
// no shared state.
func TestUnauthenticatedNotesRejected(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, "GET", "/api/notes", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/notes without token: got %d body %s, want %d",
			rec.Code, rec.Body.String(), http.StatusUnauthorized)
	}
}

// TestUnknownRouteNotFound pins the router's 404 for an undefined path.
func TestUnknownRouteNotFound(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, "GET", "/api/nope", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/nope: got %d body %s, want %d",
			rec.Code, rec.Body.String(), http.StatusNotFound)
	}
}
