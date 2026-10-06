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

// TestUnauthenticatedRejected pins 401 for auth-gated routes without a token.
func TestUnauthenticatedRejected(t *testing.T) {
	s := newTestSetup(t)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/me"},
		{"POST", "/v1/products"},
		{"POST", "/v1/orders"},
		{"GET", "/v1/orders"},
		{"GET", "/v1/orders/o-1"},
		{"POST", "/v1/posts"},
	} {
		if rec := do(t, s.handler, tc.method, tc.path, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestMalformedJSON pins 400 for invalid JSON on mutating routes.
func TestMalformedJSON(t *testing.T) {
	s := newTestSetup(t)
	register(t, s.handler, "m@example.com", "M")
	token := login(t, s.handler, "m@example.com")

	for _, tc := range []struct{ method, path, token string }{
		{"POST", "/register", ""},
		{"POST", "/login", ""},
		{"POST", "/v1/products", token},
		{"POST", "/v1/orders", token},
		{"POST", "/v1/posts", token},
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

// TestUnknownIDs pins 404 for missing product, order, and session targets.
func TestUnknownIDs(t *testing.T) {
	s := newTestSetup(t)
	register(t, s.handler, "u@example.com", "U")
	token := login(t, s.handler, "u@example.com")

	if rec := do(t, s.handler, "GET", "/v1/products/missing", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("get unknown product = %d, want 404", rec.Code)
	}
	if rec := do(t, s.handler, "GET", "/v1/orders/missing", token, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get unknown order = %d, want 404", rec.Code)
	}
	if rec := do(t, s.handler, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{"missing"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("order unknown product = %d, want 400 body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, s.handler, "GET", "/demo/session/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("session unknown = %d, want 404", rec.Code)
	}
	if rec := do(t, s.handler, "GET", "/demo/session/missing", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("session malformed = %d, want 400", rec.Code)
	}
}

// TestValidationBoundaries pins 400/401/404/409 for bad fields.
func TestValidationBoundaries(t *testing.T) {
	s := newTestSetup(t)

	tests := []struct {
		name  string
		token string
		body  any
		path  string
		want  int
		msg   string
	}{
		{"register bad email", "", map[string]string{"email": "nope", "name": "N", "password": "password123"}, "/register", http.StatusBadRequest, "invalid email"},
		{"register short password", "", map[string]string{"email": "a@b.c", "name": "N", "password": "x"}, "/register", http.StatusBadRequest, "password too short"},
		{"register missing name", "", map[string]string{"email": "a@b.c", "password": "password123"}, "/register", http.StatusBadRequest, "name required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, s.handler, "POST", tc.path, tc.token, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s = %d, want %d body %s", tc.name, rec.Code, tc.want, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.msg) {
				t.Fatalf("%s body = %s, want %q", tc.name, rec.Body.String(), tc.msg)
			}
		})
	}

	register(t, s.handler, "shop@example.com", "Shop")
	token := login(t, s.handler, "shop@example.com")

	rec := do(t, s.handler, "POST", "/login", "", map[string]string{"email": "shop@example.com", "password": "wrongpass1"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad login = %d, want 401", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/v1/products", token, map[string]any{"name": "Orphan"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("orphan product = %d, want 400", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty order = %d, want 400", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/v1/posts", token, map[string]string{"title": "", "body": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty post title = %d, want 400", rec.Code)
	}
	rec = do(t, s.handler, "GET", "/demo/secrets/MISSING_XYZ", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing secret = %d, want 404", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/demo/geo/geocode", "", map[string]string{"address": "Nowhere-xyz"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown geocode = %d, want 404", rec.Code)
	}
}

// TestDemoErrorPaths pins error contracts for demo batteries.
func TestDemoErrorPaths(t *testing.T) {
	s := newTestSetup(t)
	h := s.handler

	rec := do(t, h, "POST", "/demo/crypto/decrypt", "", map[string]string{"ciphertext": "!!!not-base64!!!"})
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusInternalServerError {
		t.Errorf("bad decrypt = %d, want 400/500 body %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/vector/query", "", map[string]any{"vector": []float32{0.1}, "top_k": 5})
	if rec.Code != http.StatusOK {
		t.Errorf("vector dim mismatch = %d, want 200 with empty matches body %s", rec.Code, rec.Body.String())
	} else if !strings.Contains(rec.Body.String(), "matches") {
		t.Errorf("vector body = %s, want matches key", rec.Body.String())
	}
	rec = do(t, h, "POST", "/demo/permission/check", "", map[string]string{
		"subject": "u1", "role": "member", "resource": "product", "action": "product.delete",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("permission check: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "allowed") {
		t.Errorf("permission body = %s, want allowed key", rec.Body.String())
	}
	rec = do(t, h, "GET", "/demo/i18n/xx/hello", "", nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("i18n unknown locale = %d, want 200/404", rec.Code)
	}
}

// TestDoubleRegisterConflict pins second register returns 409.
func TestDoubleRegisterConflict(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "dup@example.com", "Dup")
	rec := do(t, s.handler, "POST", "/register", "", map[string]string{
		"email": "dup@example.com", "name": "Dup2", "password": "password123",
	})
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate register = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "already registered") {
		t.Errorf("duplicate body = %s, want already registered", rec.Body.String())
	}
}
