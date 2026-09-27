package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/examples/showcase/internal/api"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
	"github.com/zenta-dev/zever/examples/showcase/internal/testsetup"
)

const testJWTSecret = "test-secret-for-showcase-32-bytes-min"

// testPassword is the password every test account registers with.
const testPassword = "password123"

// testSetup bundles the handler with the services tests assert against.
type testSetup struct {
	handler http.Handler
	queue   queue.Queue
	db      db.DB
}

// newTestSetup builds a fresh sqlite-backed API per test.
func newTestSetup(t *testing.T) testSetup {
	t.Helper()

	testsetup.RegisterDefaults()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Auth.Options.JWT.Secret = testJWTSecret
	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: api.LocalesFS, Dir: "locales", Fallback: "en"}
	cfg.Permission.Adapter = "rbac"
	cfg.Permission.Options.Rules = []permission.Rule{
		{Role: "admin", Action: "product.delete"},
		{Role: "user", Action: "product.delete", OwnedOnly: true, OwnedAttr: "owner"},
	}

	c := container.New(cfg)

	ctx := t.Context()
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
	limiter, err := c.RateLimit()
	if err != nil {
		t.Fatalf("RateLimit: %v", err)
	}
	perm, err := c.Permission()
	if err != nil {
		t.Fatalf("Permission: %v", err)
	}
	q, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	i18nInst, err := c.I18n()
	if err != nil {
		t.Fatalf("I18n: %v", err)
	}
	flags, err := c.Flag()
	if err != nil {
		t.Fatalf("Flag: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	if err := seed.EnsureJoinTable(ctx, database); err != nil {
		t.Fatalf("join table: %v", err)
	}

	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	api.New(database, authInst, hasher, limiter, perm, q, i18nInst, flags).Routes(r)
	return testSetup{handler: r, queue: q, db: database}
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

func register(t *testing.T, h http.Handler, email, name string) {
	t.Helper()
	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": email, "name": name, "password": testPassword,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: status %d: %s", email, rec.Code, rec.Body.String())
	}
}

func login(t *testing.T, h http.Handler, email string) string {
	t.Helper()
	rec := do(t, h, "POST", "/api/login", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status %d: %s", email, rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if out["token"] == "" {
		t.Fatalf("empty token for %s", email)
	}
	return out["token"]
}

// makeAdmin elevates a user by email to the admin role.
func makeAdmin(t *testing.T, s testSetup, email string) {
	t.Helper()
	n, err := s.db.Exec(t.Context(), `UPDATE users SET role = 'admin' WHERE email = ?`, email)
	if err != nil {
		t.Fatalf("elevate: %v", err)
	}
	if n != 1 {
		t.Fatalf("elevate %s: updated %d rows", email, n)
	}
}

// makeCategory inserts one category directly.
func makeCategory(t *testing.T, s testSetup, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(t.Context(),
		`INSERT INTO categories (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		id, name, name+" desc", now); err != nil {
		t.Fatalf("category: %v", err)
	}
}

// makeProduct creates a product as admin and returns its id.
func makeProduct(t *testing.T, s testSetup, admin, category, sku string) string {
	t.Helper()
	rec := do(t, s.handler, "POST", "/api/products", admin, map[string]any{
		"category_id": category, "sku": sku, "headline": sku + " headline",
		"description": "desc", "price_cents": 2500, "stock": 10, "weight": 1.5, "featured": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status %d: %s", sku, rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode product: %v", err)
	}
	return out["id"]
}

func TestAuthFlow(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "ann@example.com", "Ann")

	// Duplicate email conflicts.
	rec := do(t, s.handler, "POST", "/api/register", "", map[string]string{
		"email": "ann@example.com", "name": "Ann2", "password": testPassword,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: status %d, want 409", rec.Code)
	}

	// Bad password rejected.
	rec = do(t, s.handler, "POST", "/api/login", "", map[string]string{
		"email": "ann@example.com", "password": "wrong",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: status %d, want 401", rec.Code)
	}

	token := login(t, s.handler, "ann@example.com")

	rec = do(t, s.handler, "GET", "/api/me", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: status %d: %s", rec.Code, rec.Body.String())
	}
	var me map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me["email"] != "ann@example.com" || me["name"] != "Ann" {
		t.Fatalf("me = %v, want ann's profile", me)
	}

	// Missing token rejected.
	rec = do(t, s.handler, "GET", "/api/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon me: status %d, want 401", rec.Code)
	}
}

func TestProductFlow(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "admin@example.com", "Admin")
	register(t, s.handler, "bob@example.com", "Bob")
	makeAdmin(t, s, "admin@example.com")
	makeCategory(t, s, "cat-1", "Gadgets")
	admin := login(t, s.handler, "admin@example.com")
	bob := login(t, s.handler, "bob@example.com")

	// Non-admin cannot create (403 before any category check).
	rec := do(t, s.handler, "POST", "/api/products", bob, map[string]any{
		"category_id": "cat-1", "sku": "W-1", "headline": "Widget", "price_cents": 2500, "stock": 10,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member create: status %d, want 403", rec.Code)
	}

	// Unknown category rejected.
	rec = do(t, s.handler, "POST", "/api/products", admin, map[string]any{
		"category_id": "nope", "sku": "W-1", "headline": "Widget", "price_cents": 2500, "stock": 10,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad category: status %d, want 400", rec.Code)
	}

	id := makeProduct(t, s, admin, "cat-1", "W-1")

	// Duplicate sku conflicts (schema errors: already_exists).
	rec = do(t, s.handler, "POST", "/api/products", admin, map[string]any{
		"category_id": "cat-1", "sku": "W-1", "headline": "Dupe", "price_cents": 100, "stock": 1,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("dupe sku: status %d, want 409", rec.Code)
	}

	// Get + list.
	rec = do(t, s.handler, "GET", "/api/products/"+id, admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "GET", "/api/products", admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d: %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	// PUT then PATCH.
	rec = do(t, s.handler, "PUT", "/api/products/"+id, admin, map[string]string{
		"headline": "Widget v2", "description": "new",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "PATCH", "/api/products/"+id, admin, map[string]any{"stock": 7})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "PATCH", "/api/products/"+id, admin, map[string]any{"stock": -1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative stock: status %d, want 400", rec.Code)
	}

	// Member cannot delete (permission: product.delete denied).
	rec = do(t, s.handler, "DELETE", "/api/products/"+id, bob, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member delete: status %d, want 403", rec.Code)
	}

	// Admin deletes, then get 404s.
	rec = do(t, s.handler, "DELETE", "/api/products/"+id, admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin delete: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "GET", "/api/products/"+id, admin, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get deleted: status %d, want 404", rec.Code)
	}
}

func TestCheckoutAndReviews(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "admin@example.com", "Admin")
	register(t, s.handler, "shop@example.com", "Shop")
	makeAdmin(t, s, "admin@example.com")
	makeCategory(t, s, "cat-1", "Gadgets")
	admin := login(t, s.handler, "admin@example.com")
	token := login(t, s.handler, "shop@example.com")
	pid := makeProduct(t, s, admin, "cat-1", "W-9")

	// Empty cart rejected.
	rec := do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{"items": []any{}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty checkout: status %d, want 400", rec.Code)
	}

	// Unknown product surfaces 404.
	rec = do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{
		"items": []any{map[string]any{"product_id": "missing", "quantity": 1}},
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing product: status %d, want 404", rec.Code)
	}

	// Real checkout: 2 x 2500 = 5000.
	rec = do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{
		"coupon": "SAVE10", "gift_note": "hi",
		"items": []any{map[string]any{"product_id": pid, "quantity": 2}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout: status %d: %s", rec.Code, rec.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt["total_cents"] != float64(5000) {
		t.Fatalf("total = %v, want 5000", receipt["total_cents"])
	}
	if receipt["order_id"] == "" || receipt["message"] == "" || receipt["version"] != "v1" {
		t.Fatalf("receipt = %v, want order_id/message/version", receipt)
	}
	orderID, _ := receipt["order_id"].(string)

	// Stock decremented 10 -> 8.
	rec = do(t, s.handler, "GET", "/api/products/"+pid, admin, nil)
	var prod map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &prod); err != nil {
		t.Fatalf("decode product: %v", err)
	}
	if prod["stock"] != float64(8) {
		t.Fatalf("stock = %v, want 8", prod["stock"])
	}

	// Oversell conflicts.
	rec = do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{
		"items": []any{map[string]any{"product_id": pid, "quantity": 99}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("oversell: status %d, want 409", rec.Code)
	}

	// Owner reads; admin (non-owner) gets 403.
	rec = do(t, s.handler, "GET", "/api/orders/"+orderID, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner get: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "GET", "/api/orders/"+orderID, admin, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner get: status %d, want 403", rec.Code)
	}
	rec = do(t, s.handler, "GET", "/api/orders", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list orders: status %d: %s", rec.Code, rec.Body.String())
	}

	// Reviews: bad rating rejected, good one stored and listed.
	rec = do(t, s.handler, "POST", "/api/reviews", token, map[string]any{
		"product_id": pid, "rating": 9, "body": "x",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad rating: status %d, want 400", rec.Code)
	}
	rec = do(t, s.handler, "POST", "/api/reviews", token, map[string]any{
		"product_id": pid, "rating": 5, "body": "great widget",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("review: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "GET", "/api/reviews?product_id="+pid, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list reviews: status %d: %s", rec.Code, rec.Body.String())
	}
	var reviews []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &reviews); err != nil {
		t.Fatalf("decode reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("reviews len = %d, want 1", len(reviews))
	}
}

// TestCheckoutConcurrentOversellGuarded proves the checkout race fix: N
// concurrent checkouts against a product with stock for exactly one of them
// must never let more than one succeed, and stock must never go negative.
// Before the fix, handleCheckout read stock, validated in Go, then
// decremented later with no transaction and no `WHERE stock >= ?` guard --
// two concurrent requests could both pass the read-time check and both
// decrement.
func TestCheckoutConcurrentOversellGuarded(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "admin@example.com", "Admin")
	register(t, s.handler, "shop@example.com", "Shop")
	makeAdmin(t, s, "admin@example.com")
	makeCategory(t, s, "cat-race", "Gadgets")
	admin := login(t, s.handler, "admin@example.com")
	token := login(t, s.handler, "shop@example.com")
	pid := makeProduct(t, s, admin, "cat-race", "RACE-1")

	// Stock down to exactly 1 unit: only one concurrent 1-unit checkout can
	// legitimately succeed.
	rec := do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{
		"items": []any{map[string]any{"product_id": pid, "quantity": 9}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("prime checkout: status %d: %s", rec.Code, rec.Body.String())
	}

	const concurrency = 8

	codes := make([]int, concurrency)

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got := do(t, s.handler, "POST", "/api/orders/checkout", token, map[string]any{
				"items": []any{map[string]any{"product_id": pid, "quantity": 1}},
			})
			codes[i] = got.Code
		}(i)
	}
	wg.Wait()

	successes := 0
	for idx, code := range codes {
		switch code {
		case http.StatusCreated:
			successes++
		case http.StatusConflict, http.StatusInternalServerError:
			// StatusInternalServerError is tolerated here only for sqlite
			// lock-contention errors under concurrent writers, not for a
			// logic bug -- the invariant this test actually guards is
			// "successes never exceeds available stock", checked below
			// regardless of how the losing requests failed.
		default:
			t.Fatalf("checkout[%d] unexpected status %d", idx, code)
		}
	}

	if successes != 1 {
		t.Fatalf("successes = %d, want exactly 1 (oversold if > 1)", successes)
	}

	rec = do(t, s.handler, "GET", "/api/products/"+pid, admin, nil)
	var prod map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &prod); err != nil {
		t.Fatalf("decode product: %v", err)
	}

	if prod["stock"] != float64(0) {
		t.Fatalf("stock = %v, want 0 (never negative, never under-decremented)", prod["stock"])
	}
}
