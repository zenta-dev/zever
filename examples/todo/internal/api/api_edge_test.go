package api_test

import (
	"encoding/json"
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

// mustRegister logs in and returns a token for email.
func mustRegister(t *testing.T, h http.Handler, email string) string {
	t.Helper()

	if rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": email, "password": "password123",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body.String())
	}
	rec := do(t, h, "POST", "/api/login", "", map[string]string{
		"email": email, "password": "password123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("token: %v %s", err, rec.Body.String())
	}
	return out.Token
}

// TestUnauthenticatedRejected pins 401 for note routes without a token.
func TestUnauthenticatedRejected(t *testing.T) {
	h := newTestHandler(t)

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/notes"},
		{"GET", "/api/notes"},
		{"GET", "/api/notes/n-1"},
		{"PATCH", "/api/notes/n-1"},
		{"DELETE", "/api/notes/n-1"},
	} {
		if rec := do(t, h, tc.method, tc.path, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

// TestMalformedJSON pins 400 for invalid JSON on register, login, notes.
func TestMalformedJSON(t *testing.T) {
	h := newTestHandler(t)
	token := mustRegister(t, h, "m@example.com")

	rec := do(t, h, "POST", "/api/notes", token, map[string]string{"title": "seed", "body": "b"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed note: %d", rec.Code)
	}
	var seeded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &seeded); err != nil {
		t.Fatalf("seed id: %v", err)
	}

	for _, tc := range []struct{ method, path, token string }{
		{"POST", "/api/register", ""},
		{"POST", "/api/login", ""},
		{"POST", "/api/notes", token},
		{"PATCH", "/api/notes/" + seeded.ID, token},
	} {
		rec := doRaw(t, h, tc.method, tc.path, tc.token, "{bad-json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "invalid JSON") {
			t.Errorf("%s %s body = %s, want invalid JSON", tc.method, tc.path, rec.Body.String())
		}
	}
}

// TestUnknownIDs pins 404 for missing note targets.
func TestUnknownIDs(t *testing.T) {
	h := newTestHandler(t)
	token := mustRegister(t, h, "u@example.com")

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/notes/does-not-exist"},
		{"PATCH", "/api/notes/does-not-exist"},
		{"DELETE", "/api/notes/does-not-exist"},
	} {
		var body any
		if tc.method == "PATCH" {
			body = map[string]any{"title": "x"}
		}
		if rec := do(t, h, tc.method, tc.path, token, body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404 body %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestValidationBoundaries pins 400 for empty titles and bad register fields.
func TestValidationBoundaries(t *testing.T) {
	h := newTestHandler(t)
	token := mustRegister(t, h, "v@example.com")

	tests := []struct {
		name   string
		method string
		path   string
		token  string
		body   any
		want   string
	}{
		{"register bad email", "POST", "/api/register", "", map[string]string{"email": "nope", "password": "password123"}, "invalid email"},
		{"register short password", "POST", "/api/register", "", map[string]string{"email": "a@b.c", "password": "x"}, "password too short"},
		{"note empty title", "POST", "/api/notes", token, map[string]string{"title": "  ", "body": "b"}, "title required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path, tc.token, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want 400 %s", tc.name, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%s body = %s, want %q", tc.name, rec.Body.String(), tc.want)
			}
		})
	}

	rec := do(t, h, "POST", "/api/notes", token, map[string]string{"title": "ok", "body": "b"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rec = do(t, h, "PATCH", "/api/notes/"+created.ID, token, map[string]any{"title": "  "})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("patch empty title = %d, want 400", rec.Code)
	}
}

// TestDoubleDeleteNotFound pins second delete returns 404.
func TestDoubleDeleteNotFound(t *testing.T) {
	h := newTestHandler(t)
	token := mustRegister(t, h, "d@example.com")

	rec := do(t, h, "POST", "/api/notes", token, map[string]string{"title": "t", "body": "b"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec := do(t, h, "DELETE", "/api/notes/"+created.ID, token, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("first delete = %d, want 204", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/notes/"+created.ID, token, nil); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}
