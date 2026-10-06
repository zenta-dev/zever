package api_test

import (
	"bytes"
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
	"github.com/zenta-dev/zever/examples/todo/internal/testsetup"
)

// benchHandler builds a seeded handler and token for benchmarks.
func benchHandler(b *testing.B) (http.Handler, string) {
	b.Helper()

	testsetup.RegisterDefaults()
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(b.TempDir(), "bench.db")
	cfg.Auth.Options.JWT.Secret = testJWTSecret
	c := container.New(cfg)
	b.Cleanup(func() { _ = c.Close(b.Context()) })

	database, err := c.DB()
	if err != nil {
		b.Fatalf("DB: %v", err)
	}
	authInst, err := c.Auth()
	if err != nil {
		b.Fatalf("Auth: %v", err)
	}
	hasher, err := c.Password()
	if err != nil {
		b.Fatalf("Password: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		b.Fatalf("Router: %v", err)
	}
	raw, err := os.ReadFile("testdata/schema.sql")
	if err != nil {
		b.Fatalf("schema: %v", err)
	}
	ctx := b.Context()
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			b.Fatalf("migrate: %v", err)
		}
	}
	api.New(database, authInst, hasher).Routes(r)

	post := func(path, token string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			b.Fatalf("encode: %v", err)
		}
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("/api/register", "", map[string]string{"email": "bench@example.com", "password": "password123"}); rec.Code != http.StatusCreated {
		b.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	rec := post("/api/login", "", map[string]string{"email": "bench@example.com", "password": "password123"})
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		b.Fatalf("token: %v", err)
	}
	for i := 0; i < 10; i++ {
		if rec := post("/api/notes", out.Token, map[string]string{"title": "t", "body": "b"}); rec.Code != http.StatusCreated {
			b.Fatalf("create: %d", rec.Code)
		}
	}
	return r, out.Token
}

// BenchmarkListNotes measures the owned-note listing hot path.
func BenchmarkListNotes(b *testing.B) {
	h, token := benchHandler(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/notes", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("list: %d", rec.Code)
		}
	}
}
