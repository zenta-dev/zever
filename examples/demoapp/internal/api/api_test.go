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
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/examples/demoapp/internal/api"
	"github.com/zenta-dev/zever/examples/demoapp/locales"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/permission"
)

const testJWTSecret = "test-secret-for-demoapp-32-bytes-min"

// testPassword is the password every test account registers with.
const testPassword = "password123"

// testSetup bundles the handler with the services tests assert against.
type testSetup struct {
	handler http.Handler
	db      db.DB
}

// newTestSetup builds a fresh sqlite-backed API per test. Storage, media,
// search, and vector indexes live under per-test temp dirs/files so tests
// never touch the working tree.
func newTestSetup(t *testing.T) testSetup {
	t.Helper()

	tmp := t.TempDir()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(tmp, "test.db")
	cfg.Auth.Options.JWT.Secret = testJWTSecret
	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: locales.FS, Dir: ".", Fallback: "en"}
	cfg.Permission.Adapter = "rbac"
	cfg.Permission.Options.Rules = []permission.Rule{
		{Role: "admin", Action: "*"},
		{Role: "member", Action: "product.read"},
		{Role: "member", Action: "order.create"},
		{Role: "member", Action: "post.create"},
		{Role: "member", Action: "post.read"},
	}
	flagPath, err := filepath.Abs(filepath.Join("..", "..", "flags.json"))
	if err != nil {
		t.Fatalf("flag path: %v", err)
	}
	cfg.Flag.Options.Static.Path = flagPath
	geoPath, err := filepath.Abs(filepath.Join("..", "..", "data", "cities.json"))
	if err != nil {
		t.Fatalf("geo path: %v", err)
	}
	cfg.Geo.Options.Path = geoPath
	cfg.Search.Options.DSN = filepath.Join(tmp, "search.db")
	cfg.VectorStore.Options.DSN = filepath.Join(tmp, "vectors.db")
	cfg.VectorStore.Options.Dimension = 8
	cfg.Storage.Options.Root = filepath.Join(tmp, "storage")
	cfg.Media.Options.Root = filepath.Join(tmp, "media")

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
	logger, err := c.Log()
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	d := api.Deps{DB: database, Auth: authInst, Password: hasher, Log: logger}
	d.Cache, _ = c.Cache()
	d.Flag, _ = c.Flag()
	d.Permission, _ = c.Permission()
	d.RateLimit, _ = c.RateLimit()
	d.Lock, _ = c.Lock()
	d.Idempotency, _ = c.Idempotency()
	d.Session, _ = c.Session()
	d.Queue, _ = c.Queue()
	d.Job, _ = c.Job()
	d.EventBus, _ = c.EventBus()
	d.Search, _ = c.Search()
	d.VectorStore, _ = c.VectorStore()
	d.Storage, _ = c.Storage()
	d.Media, _ = c.Media()
	d.AI, _ = c.AI()
	d.Geo, _ = c.Geo()
	d.I18n, _ = c.I18n()
	d.Crypto, _ = c.Crypto()
	d.Secrets, _ = c.Secrets()
	d.Notification, _ = c.Notification()
	d.Mailer, _ = c.Mailer()
	d.Webhook, _ = c.Webhook()
	d.Workflow, _ = c.Workflow()
	d.Observability, _ = c.Observability()
	d.Analytics, _ = c.Analytics()
	d.Payment, _ = c.Payment()
	d.Billing, _ = c.Billing()
	d.Document, _ = c.Document()
	d.Tenant, _ = c.Tenant()

	if d.Cache == nil {
		t.Fatal("Cache not resolved")
	}
	if d.Search == nil {
		t.Fatal("Search not resolved")
	}
	if d.VectorStore == nil {
		t.Fatal("VectorStore not resolved")
	}
	if d.Storage == nil {
		t.Fatal("Storage not resolved")
	}
	if d.Media == nil {
		t.Fatal("Media not resolved")
	}
	if d.Geo == nil {
		t.Fatal("Geo not resolved")
	}
	if d.I18n == nil {
		t.Fatal("I18n not resolved")
	}
	if d.Crypto == nil {
		t.Fatal("Crypto not resolved")
	}
	if d.Secrets == nil {
		t.Fatal("Secrets not resolved")
	}
	if d.Workflow == nil {
		t.Fatal("Workflow not resolved")
	}
	api.RegisterDemoWorkflow(d.Workflow)

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

	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	api.New(d).Routes(r)
	return testSetup{handler: r, db: database}
}

// do sends a request with an optional bearer token. A nil body sends no
// payload; a string body sends raw bytes; anything else sends JSON.
func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	var contentType string
	switch b := body.(type) {
	case nil:
	case string:
		buf.WriteString(b)
	default:
		contentType = "application/json"
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, &buf)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
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
	rec := do(t, h, "POST", "/register", "", map[string]string{
		"email": email, "name": name, "password": testPassword,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: status %d: %s", email, rec.Code, rec.Body.String())
	}
}

func login(t *testing.T, h http.Handler, email string) string {
	t.Helper()
	rec := do(t, h, "POST", "/login", "", map[string]string{
		"email": email, "password": testPassword,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status %d: %s", email, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("empty token for %s", email)
	}
	return token
}

// makeCategory inserts one category directly.
func makeCategory(t *testing.T, s testSetup, id, name string) {
	t.Helper()
	if _, err := s.db.Exec(t.Context(),
		`INSERT INTO categories (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		id, name, name+" desc", "2026-01-02T03:04:05Z"); err != nil {
		t.Fatalf("category: %v", err)
	}
}

// makeProduct creates a product and returns its id.
func makeProduct(t *testing.T, h http.Handler, token, category, name string, price int64) string {
	t.Helper()
	rec := do(t, h, "POST", "/v1/products", token, map[string]any{
		"category_id": category, "name": name, "description": "desc",
		"price_cents": price, "stock": 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product %s: status %d: %s", name, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode product: %v", err)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("empty product id: %s", rec.Body.String())
	}
	return id
}

func TestHealth(t *testing.T) {
	s := newTestSetup(t)
	rec := do(t, s.handler, "GET", "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("health: status %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["status"] != "ok" {
		t.Fatalf("health body = %s", rec.Body.String())
	}
}

func TestAuthFlow(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "ann@example.com", "Ann")

	rec := do(t, s.handler, "POST", "/register", "", map[string]string{
		"email": "ann@example.com", "name": "Ann2", "password": testPassword,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: status %d, want 409", rec.Code)
	}

	rec = do(t, s.handler, "POST", "/login", "", map[string]string{
		"email": "ann@example.com", "password": "wrongpass1",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: status %d, want 401", rec.Code)
	}

	token := login(t, s.handler, "ann@example.com")

	rec = do(t, s.handler, "GET", "/me", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: status %d: %s", rec.Code, rec.Body.String())
	}
	var me map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me["email"] != "ann@example.com" || me["name"] != "Ann" || me["role"] != "member" {
		t.Fatalf("me = %v, want ann's profile", me)
	}

	rec = do(t, s.handler, "GET", "/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon me: status %d, want 401", rec.Code)
	}
}

func TestProductOrderLifecycle(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "shop@example.com", "Shop")
	token := login(t, s.handler, "shop@example.com")

	// Product without a category and without a fallback category is 400.
	rec := do(t, s.handler, "POST", "/v1/products", token, map[string]any{"name": "Orphan"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("orphan product: status %d, want 400", rec.Code)
	}

	makeCategory(t, s, "cat-1", "Gadgets")

	pid := makeProduct(t, s.handler, token, "cat-1", "Widget", 2500)

	rec = do(t, s.handler, "GET", "/v1/products/"+pid, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get product: status %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s.handler, "GET", "/v1/products?category_id=cat-1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list products: status %d: %s", rec.Code, rec.Body.String())
	}
	var list map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list["products"]) != 1 {
		t.Fatalf("list products = %s, want 1 product", rec.Body.String())
	}

	// Empty order is rejected.
	rec = do(t, s.handler, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty order: status %d, want 400", rec.Code)
	}

	// Unknown product is rejected.
	rec = do(t, s.handler, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{"missing"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown product: status %d, want 400", rec.Code)
	}

	// Real order totals the product price.
	rec = do(t, s.handler, "POST", "/v1/orders", token, map[string]any{"product_ids": []string{pid}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: status %d: %s", rec.Code, rec.Body.String())
	}
	var order map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order["total_cents"] != float64(2500) || order["status"] != "pending" {
		t.Fatalf("order = %v, want total 2500 pending", order)
	}
	orderID, _ := order["id"].(string)

	rec = do(t, s.handler, "GET", "/v1/orders", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list orders: status %d: %s", rec.Code, rec.Body.String())
	}
	var orders map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &orders); err != nil || len(orders["orders"]) != 1 {
		t.Fatalf("list orders = %s, want 1 order", rec.Body.String())
	}

	rec = do(t, s.handler, "GET", "/v1/orders/"+orderID, token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get order: status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOwnerScoping(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "alice@example.com", "Alice")
	register(t, s.handler, "bob@example.com", "Bob")
	alice := login(t, s.handler, "alice@example.com")
	bob := login(t, s.handler, "bob@example.com")
	makeCategory(t, s, "cat-1", "Gadgets")
	pid := makeProduct(t, s.handler, alice, "cat-1", "Widget", 2500)

	rec := do(t, s.handler, "POST", "/v1/orders", alice, map[string]any{"product_ids": []string{pid}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice order: status %d: %s", rec.Code, rec.Body.String())
	}
	var order map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	orderID, _ := order["id"].(string)

	// Bob cannot read Alice's order; it looks missing to him.
	rec = do(t, s.handler, "GET", "/v1/orders/"+orderID, bob, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob get foreign order: status %d, want 404", rec.Code)
	}

	// Bob's order list is empty.
	rec = do(t, s.handler, "GET", "/v1/orders", bob, nil)
	var orders map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &orders); err != nil || len(orders["orders"]) != 0 {
		t.Fatalf("bob orders = %s, want empty", rec.Body.String())
	}
}

func TestTamperedToken(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "alice@example.com", "Alice")
	token := login(t, s.handler, "alice@example.com")

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	// Flip the first char of the signature segment: its six bits are all
	// significant, so the decoded signature always changes.
	sig := parts[2]
	first := byte('A')
	if sig[0] == 'A' {
		first = 'B'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(first) + sig[1:]
	rec := do(t, s.handler, "GET", "/me", tampered, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered: got %d body %s, want 401", rec.Code, rec.Body.String())
	}
}

func TestPosts(t *testing.T) {
	s := newTestSetup(t)

	register(t, s.handler, "blog@example.com", "Blog")
	token := login(t, s.handler, "blog@example.com")

	rec := do(t, s.handler, "POST", "/v1/posts", "", map[string]string{"title": "Hi", "body": "x"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anon post: status %d, want 401", rec.Code)
	}

	rec = do(t, s.handler, "POST", "/v1/posts", token, map[string]string{"title": "Hello", "body": "world"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create post: status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Crypto") != "encrypted" {
		t.Fatalf("create post: want X-Crypto header, got %v", rec.Header())
	}

	rec = do(t, s.handler, "GET", "/v1/posts", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list posts: status %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out["posts"]) != 1 {
		t.Fatalf("list posts = %s, want 1 post", rec.Body.String())
	}
	if out["posts"][0]["title"] != "Hello" {
		t.Fatalf("post = %v, want Hello", out["posts"][0])
	}
}

func TestDemoBatteries(t *testing.T) {
	s := newTestSetup(t)
	h := s.handler

	t.Run("cache", func(t *testing.T) {
		if rec := do(t, h, "GET", "/demo/cache/hello", "", nil); rec.Code != http.StatusNotFound {
			t.Fatalf("cache miss: status %d, want 404", rec.Code)
		}
		if rec := do(t, h, "POST", "/demo/cache/hello", "", "world"); rec.Code != http.StatusOK {
			t.Fatalf("cache set: status %d: %s", rec.Code, rec.Body.String())
		}
		rec := do(t, h, "GET", "/demo/cache/hello", "", nil)
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["value"] != "world" {
			t.Fatalf("cache get = %s, want world", rec.Body.String())
		}
	})

	t.Run("flag", func(t *testing.T) {
		rec := do(t, h, "GET", "/demo/flag/new_checkout", "", nil)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["value"] != true {
			t.Fatalf("flag = %s, want true", rec.Body.String())
		}
	})

	t.Run("permission", func(t *testing.T) {
		allowed := func(role, action, resource string) bool {
			t.Helper()
			rec := do(t, h, "POST", "/demo/permission/check", "", map[string]string{
				"subject": "u1", "role": role, "resource": resource, "action": action,
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("check: status %d: %s", rec.Code, rec.Body.String())
			}
			var out map[string]bool
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			return out["allowed"]
		}
		if !allowed("member", "order.create", "order") {
			t.Fatal("member order.create denied, want allow")
		}
		if allowed("member", "product.delete", "product") {
			t.Fatal("member product.delete allowed, want deny")
		}
		if !allowed("admin", "product.delete", "product") {
			t.Fatal("admin product.delete denied, want allow")
		}
	})

	t.Run("ratelimit", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/ratelimit/test-key", "", nil)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["allowed"] != true {
			t.Fatalf("ratelimit = %s, want allowed", rec.Body.String())
		}
	})

	t.Run("lock", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/lock/test-key", "", nil)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["acquired"] != true {
			t.Fatalf("lock = %s, want acquired", rec.Body.String())
		}
	})

	t.Run("idempotency", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/idempotency/k1", "", map[string]string{"data": "payload"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("first: status %d: %s", rec.Code, rec.Body.String())
		}
		rec = do(t, h, "POST", "/demo/idempotency/k1", "", map[string]string{"data": "payload"})
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["replay"] != true {
			t.Fatalf("second = %s, want replay", rec.Body.String())
		}
	})

	t.Run("session", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/session", "", map[string]any{"user": "ann"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: status %d: %s", rec.Code, rec.Body.String())
		}
		var created map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created["id"] == "" {
			t.Fatalf("create = %s, want id", rec.Body.String())
		}
		rec = do(t, h, "GET", "/demo/session/"+created["id"], "", nil)
		var data map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil || data["user"] != "ann" {
			t.Fatalf("get = %s, want user ann", rec.Body.String())
		}
		if rec := do(t, h, "GET", "/demo/session/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", nil); rec.Code != http.StatusNotFound {
			t.Fatalf("miss: status %d, want 404", rec.Code)
		}
		if rec := do(t, h, "GET", "/demo/session/missing", "", nil); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed: status %d, want 400", rec.Code)
		}
	})

	t.Run("queue", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/queue/test-topic", "", "hello")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("push: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("eventbus", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/eventbus/publish", "", map[string]string{
			"topic": "demo.test", "payload": "hi",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("publish: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("search", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/search/index", "", map[string]string{
			"id": "doc-1", "content": "zenbook laptop for builders",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("index: status %d: %s", rec.Code, rec.Body.String())
		}
		rec = do(t, h, "GET", "/demo/search?q=zenbook", "", nil)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		hits, _ := out["Hits"].([]any)
		if len(hits) == 0 {
			t.Fatalf("search = %s, want hits", rec.Body.String())
		}
	})

	t.Run("vector", func(t *testing.T) {
		vec := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
		rec := do(t, h, "POST", "/demo/vector/upsert", "", map[string]any{"id": "v1", "vector": vec})
		if rec.Code != http.StatusOK {
			t.Fatalf("upsert: status %d: %s", rec.Code, rec.Body.String())
		}
		rec = do(t, h, "POST", "/demo/vector/query", "", map[string]any{"vector": vec, "top_k": 5})
		var out map[string][]map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out["matches"]) == 0 {
			t.Fatalf("query = %s, want matches", rec.Body.String())
		}
		if out["matches"][0]["ID"] != "v1" {
			t.Fatalf("top match = %v, want v1", out["matches"][0])
		}
	})

	t.Run("storage", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/storage/presign", "", map[string]string{
			"bucket": "demo", "key": "a/b.bin",
		})
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["url"] == "" {
			t.Fatalf("presign = %s, want url", rec.Body.String())
		}
	})

	t.Run("media", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/media/upload", "", "bytes")
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["ID"] == "" {
			t.Fatalf("upload = %s, want id", rec.Body.String())
		}
	})

	t.Run("geo", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/geo/geocode", "", map[string]string{"address": "Springfield"})
		var out map[string][]map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out["results"]) == 0 {
			t.Fatalf("geocode = %s, want results", rec.Body.String())
		}
		if rec := do(t, h, "POST", "/demo/geo/geocode", "", map[string]string{"address": "Nowhere-xyz"}); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown: status %d, want 404", rec.Code)
		}
	})

	t.Run("i18n", func(t *testing.T) {
		rec := do(t, h, "GET", "/demo/i18n/en/hello", "", nil)
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["message"] != "Hello, Demo!" {
			t.Fatalf("en = %s, want Hello, Demo!", rec.Body.String())
		}
		rec = do(t, h, "GET", "/demo/i18n/fr/hello", "", nil)
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["message"] == "" {
			t.Fatalf("fr = %s, want french message", rec.Body.String())
		}
	})

	t.Run("crypto", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/crypto/encrypt", "", map[string]string{"plaintext": "hello"})
		var enc map[string][]byte
		if err := json.Unmarshal(rec.Body.Bytes(), &enc); err != nil || len(enc["ciphertext"]) == 0 {
			t.Fatalf("encrypt = %s, want ciphertext", rec.Body.String())
		}
		rec = do(t, h, "POST", "/demo/crypto/decrypt", "", map[string][]byte{"ciphertext": enc["ciphertext"]})
		var dec map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &dec); err != nil || dec["plaintext"] != "hello" {
			t.Fatalf("decrypt = %s, want hello", rec.Body.String())
		}
	})

	t.Run("secrets", func(t *testing.T) {
		t.Setenv("ZEVER_DEMO_NOTE", "hi")
		rec := do(t, h, "GET", "/demo/secrets/DEMO_NOTE", "", nil)
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["value"] != "hi" {
			t.Fatalf("secret = %s, want hi", rec.Body.String())
		}
		if rec := do(t, h, "GET", "/demo/secrets/MISSING_XYZ", "", nil); rec.Code != http.StatusNotFound {
			t.Fatalf("miss: status %d, want 404", rec.Code)
		}
	})

	t.Run("notification", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/notification", "", map[string]string{
			"target": "dev", "title": "Hi", "body": "test",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("notify: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("webhook", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/webhook/register", "", map[string]string{
			"event": "demo.event", "target": "https://example.com/hook",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("register: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("workflow", func(t *testing.T) {
		rec := do(t, h, "POST", "/demo/workflow/start", "", map[string]any{"name": "demo-workflow"})
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["run_id"] == "" {
			t.Fatalf("start = %s, want run_id", rec.Body.String())
		}
	})

	t.Run("tenant", func(t *testing.T) {
		if rec := do(t, h, "GET", "/demo/tenant", "", nil); rec.Code != http.StatusOK {
			t.Fatalf("tenant: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("observability", func(t *testing.T) {
		if rec := do(t, h, "GET", "/demo/observability", "", nil); rec.Code != http.StatusOK {
			t.Fatalf("observability: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("jobs", func(t *testing.T) {
		if err := job.Register("demoapp-test-noop", func(_ context.Context, _ struct{}) error { return nil }); err != nil {
			t.Logf("register noop: %v", err)
		}
		rec := do(t, h, "POST", "/demo/jobs/dispatch/demoapp-test-noop", "", map[string]any{})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("dispatch: status %d: %s", rec.Code, rec.Body.String())
		}
		rec = do(t, h, "POST", "/demo/jobs/dispatch/no-such-job", "", map[string]any{})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("unknown dispatch: status %d, want 500", rec.Code)
		}
	})

	t.Run("ai-document", func(t *testing.T) {
		for _, path := range []string{"/demo/ai/generate", "/demo/document/render"} {
			rec := do(t, h, "POST", path, "", map[string]string{})
			if rec.Code == http.StatusNotFound {
				t.Fatalf("%s: route missing", path)
			}
		}
	})
}
