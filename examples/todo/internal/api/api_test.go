package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/todo/internal/api"

	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/router/stdhttp"
)

const testJWTSecret = "test-secret-for-todo-app-32-bytes-min!"

// newTestHandler builds a fresh sqlite-backed API per test.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Auth.Options.JWT.Secret = testJWTSecret

	c := container.New(cfg)

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	authInst, err := c.Auth()
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	hasher, err := c.Password()
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	raw, err := os.ReadFile("testdata/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	ctx := t.Context()
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		_ = c.Close(ctx)
	})

	api.New(database, authInst, hasher).Routes(r)
	return r
}

// do sends a JSON request with an optional bearer token.
func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// login registers expectations around status codes.
func TestFullFlow(t *testing.T) {
	h := newTestHandler(t)

	// Register.
	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: got %d body %s", rec.Code, rec.Body.String())
	}

	// Duplicate register conflicts.
	rec = do(t, h, "POST", "/api/register", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: got %d body %s", rec.Code, rec.Body.String())
	}

	// Short password rejected.
	rec = do(t, h, "POST", "/api/register", "", map[string]string{
		"email": "short@example.com", "password": "short",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password: got %d body %s", rec.Code, rec.Body.String())
	}

	// Unauthenticated notes access rejected.
	rec = do(t, h, "GET", "/api/notes", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed: got %d body %s", rec.Code, rec.Body.String())
	}

	// Login.
	rec = do(t, h, "POST", "/api/login", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d body %s", rec.Code, rec.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil || login.Token == "" {
		t.Fatalf("login token missing: %v body %s", err, rec.Body.String())
	}

	// Bad password login rejected.
	rec = do(t, h, "POST", "/api/login", "", map[string]string{
		"email": "alice@example.com", "password": "wrongpass1",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: got %d body %s", rec.Code, rec.Body.String())
	}

	// Create note.
	rec = do(t, h, "POST", "/api/notes", login.Token, map[string]string{
		"title": "buy milk", "body": "2%",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d body %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create id missing: %v body %s", err, rec.Body.String())
	}

	// List shows one note.
	rec = do(t, h, "GET", "/api/notes", login.Token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body %s", rec.Code, rec.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 1 {
		t.Fatalf("list want 1, got %s err %v", rec.Body.String(), err)
	}

	// Patch title and done.
	rec = do(t, h, "PATCH", "/api/notes/"+created.ID, login.Token, map[string]any{
		"title": "buy oat milk", "done": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body %s", rec.Code, rec.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("patch decode: %v", err)
	}
	if patched["title"] != "buy oat milk" || patched["done"] != true {
		t.Fatalf("patch not applied: %s", rec.Body.String())
	}

	// Delete.
	rec = do(t, h, "DELETE", "/api/notes/"+created.ID, login.Token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d body %s", rec.Code, rec.Body.String())
	}

	// Deleted note is gone.
	rec = do(t, h, "GET", "/api/notes/"+created.ID, login.Token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get deleted: got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestOwnerScoping ensures one user cannot see another user's notes.
func TestOwnerScoping(t *testing.T) {
	h := newTestHandler(t)

	tokens := map[string]string{}
	for _, u := range []struct{ email, pass string }{
		{"alice@example.com", "password123"},
		{"bob@example.com", "password456"},
	} {
		rec := do(t, h, "POST", "/api/register", "", map[string]string{
			"email": u.email, "password": u.pass,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %s: got %d", u.email, rec.Code)
		}
		rec = do(t, h, "POST", "/api/login", "", map[string]string{
			"email": u.email, "password": u.pass,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: got %d", u.email, rec.Code)
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("token decode: %v", err)
		}
		tokens[u.email] = out.Token
	}

	rec := do(t, h, "POST", "/api/notes", tokens["alice@example.com"], map[string]string{
		"title": "alice private", "body": "hi",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create: got %d", rec.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Bob cannot get, patch, or delete Alice's note; all 404.
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/notes/" + created.ID},
		{"PATCH", "/api/notes/" + created.ID},
		{"DELETE", "/api/notes/" + created.ID},
	} {
		var body any
		if tc.method == "PATCH" {
			body = map[string]any{"title": "hijack"}
		}
		rec = do(t, h, tc.method, tc.path, tokens["bob@example.com"], body)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s foreign: got %d body %s", tc.method, rec.Code, rec.Body.String())
		}
	}

	// Bob's list is empty; Alice still sees her note.
	rec = do(t, h, "GET", "/api/notes", tokens["bob@example.com"], nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob list: got %d", rec.Code)
	}
	var bobNotes []any
	if err := json.Unmarshal(rec.Body.Bytes(), &bobNotes); err != nil || len(bobNotes) != 0 {
		t.Fatalf("bob list want empty, got %s", rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/notes/"+created.ID, tokens["alice@example.com"], nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("alice get: got %d", rec.Code)
	}
}

// TestTamperedToken ensures a modified JWT is rejected.
func TestTamperedToken(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: got %d", rec.Code)
	}
	rec = do(t, h, "POST", "/api/login", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	parts := strings.Split(out.Token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	// Flip the first char of the signature segment: its six bits are all
	// significant, so the decoded signature always changes. (Flipping the
	// last char is a no-op when only padding bits differ.)
	sig := parts[2]
	first := byte('A')
	if sig[0] == 'A' {
		first = 'B'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(first) + sig[1:]
	rec = do(t, h, "GET", "/api/notes", tampered, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered: got %d body %s", rec.Code, rec.Body.String())
	}
}
