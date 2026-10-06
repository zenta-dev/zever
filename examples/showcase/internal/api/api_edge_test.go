package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doRaw sends a raw body for malformed-payload cases.
func doRaw(t *testing.T, h http.Handler, method, path, token, raw string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestUnauthenticatedRejected pins 401 for protected routes without a token.
func TestUnauthenticatedRejected(t *testing.T) {
	s := newTestSetup(t)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/me"},
		{"POST", "/api/products"},
		{"GET", "/api/products/p-1"},
		{"PUT", "/api/products/p-1"},
		{"PATCH", "/api/products/p-1"},
		{"DELETE", "/api/products/p-1"},
		{"POST", "/api/orders/checkout"},
		{"GET", "/api/orders"},
		{"GET", "/api/orders/o-1"},
		{"POST", "/api/reviews"},
	} {
		rec := do(t, s.handler, tc.method, tc.path, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

// TestMalformedJSON pins 400 for invalid JSON on mutating routes.
func TestMalformedJSON(t *testing.T) {
	s := newTestSetup(t)
	register(t, s.handler, "e@example.com", "E")
	token := login(t, s.handler, "e@example.com")

	for _, tc := range []struct{ method, path, token string }{
		{"POST", "/api/register", ""},
		{"POST", "/api/login", ""},
		{"POST", "/api/products", token},
		{"POST", "/api/orders/checkout", token},
		{"POST", "/api/reviews", token},
	} {
		rec := doRaw(t, s.handler, tc.method, tc.path, tc.token, "{bad-json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "invalid JSON") {
			t.Errorf("%s %s body = %s, want invalid JSON", tc.method, tc.path, rec.Body.String())
		}
	}
}

// TestUnknownIDs pins 404 for missing product, order, and review targets.
func TestUnknownIDs(t *testing.T) {
	s := newTestSetup(t)
	register(t, s.handler, "admin@example.com", "Admin")
	makeAdmin(t, s, "admin@example.com")
	register(t, s.handler, "u@example.com", "U")
	admin := login(t, s.handler, "admin@example.com")
	user := login(t, s.handler, "u@example.com")

	rec := do(t, s.handler, "GET", "/api/products/missing", admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get unknown product = %d, want 404", rec.Code)
	}
	rec = do(t, s.handler, "GET", "/api/orders/missing", user, nil)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("get unknown order = %d, want 404/403", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/api/orders/checkout", user, map[string]any{
		"items": []any{map[string]any{"product_id": "missing", "quantity": 1}},
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("checkout unknown product = %d, want 404", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/api/reviews", user, map[string]any{
		"product_id": "missing", "rating": 5, "body": "x",
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("review unknown product = %d, want 404", rec.Code)
	}
}

// TestValidationBoundaries pins 400/403/409 for bad fields and roles.
func TestValidationBoundaries(t *testing.T) {
	s := newTestSetup(t)
	register(t, s.handler, "admin@example.com", "Admin")
	makeAdmin(t, s, "admin@example.com")
	register(t, s.handler, "bob@example.com", "Bob")
	makeCategory(t, s, "cat-1", "Gadgets")
	admin := login(t, s.handler, "admin@example.com")
	bob := login(t, s.handler, "bob@example.com")

	tests := []struct {
		name  string
		token string
		body  any
		path  string
		want  int
		msg   string
	}{
		{"register bad email", "", map[string]string{"email": "nope", "name": "N", "password": "password123"}, "/api/register", http.StatusBadRequest, "invalid email"},
		{"register short password", "", map[string]string{"email": "n@n.n", "name": "N", "password": "short"}, "/api/register", http.StatusBadRequest, "password too short"},
		{"product missing sku", admin, map[string]any{"category_id": "cat-1", "headline": "H"}, "/api/products", http.StatusBadRequest, "sku and headline required"},
		{"product negative price", admin, map[string]any{"category_id": "cat-1", "sku": "S-1", "headline": "H", "price_cents": -1}, "/api/products", http.StatusBadRequest, "negative price"},
		{"product unknown category", admin, map[string]any{"category_id": "nope", "sku": "S-1", "headline": "H"}, "/api/products", http.StatusBadRequest, "unknown category"},
		{"member create forbidden", bob, map[string]any{"category_id": "cat-1", "sku": "S-2", "headline": "H"}, "/api/products", http.StatusForbidden, "admin only"},
		{"checkout empty cart", bob, map[string]any{"items": []any{}}, "/api/orders/checkout", http.StatusBadRequest, "empty cart"},
		{"checkout bad quantity", bob, map[string]any{"items": []any{map[string]any{"product_id": "x", "quantity": 0}}}, "/api/orders/checkout", http.StatusBadRequest, "bad quantity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := "POST"
			rec := do(t, s.handler, method, tc.path, tc.token, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s = %d, want %d body %s", tc.name, rec.Code, tc.want, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.msg) {
				t.Fatalf("%s body = %s, want %q", tc.name, rec.Body.String(), tc.msg)
			}
		})
	}

	pid := makeProduct(t, s, admin, "cat-1", "BOUND-1")
	rec := do(t, s.handler, "PATCH", "/api/products/"+pid, admin, map[string]any{"stock": -5})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("patch negative stock = %d, want 400", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/api/reviews", bob, map[string]any{
		"product_id": pid, "rating": 9, "body": "x",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("review bad rating = %d, want 400", rec.Code)
	}
}
