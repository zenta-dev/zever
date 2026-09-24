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
	"github.com/zenta-dev/zever/examples/bookings/internal/api"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"

	_ "github.com/zenta-dev/zever/analytics/log"
	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/billing/stub"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/crypto/local"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/document/local"
	_ "github.com/zenta-dev/zever/eventbus/memory"
	_ "github.com/zenta-dev/zever/flag/static"
	_ "github.com/zenta-dev/zever/geo/static"
	_ "github.com/zenta-dev/zever/i18n/embed"
	_ "github.com/zenta-dev/zever/idempotency/memory"
	_ "github.com/zenta-dev/zever/lock/memory"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/mailer/log"
	_ "github.com/zenta-dev/zever/media/local"
	_ "github.com/zenta-dev/zever/notification/log"
	_ "github.com/zenta-dev/zever/observability/stdout"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/payment/stub"
	_ "github.com/zenta-dev/zever/permission/rbac"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/ratelimit/memory"
	_ "github.com/zenta-dev/zever/router/stdhttp"
	_ "github.com/zenta-dev/zever/scheduler/embedded"
	_ "github.com/zenta-dev/zever/search/sqlite"
	_ "github.com/zenta-dev/zever/secrets/env"
	_ "github.com/zenta-dev/zever/session/memory"
	_ "github.com/zenta-dev/zever/storage/local"
	_ "github.com/zenta-dev/zever/tenant/single"
	_ "github.com/zenta-dev/zever/vectorstore/sqlite"
	_ "github.com/zenta-dev/zever/webhook/queue"
	_ "github.com/zenta-dev/zever/workflow/memory"
)

const testJWTSecret = "test-secret-for-bookings-32-bytes-min!"

// testSetup bundles the handler with the queue for dispatch assertions.
type testSetup struct {
	handler http.Handler
	queue   queue.Queue
}

// newTestSetup builds a fresh sqlite-backed API per test.
func newTestSetup(t *testing.T) testSetup {
	t.Helper()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Auth.Options.JWT.Secret = testJWTSecret
	cfg.Geo.Options.Path = filepath.Join("testdata", "cities.json")
	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: api.LocalesFS, Dir: "locales", Fallback: "en"}
	cfg.Permission.Adapter = "rbac"
	cfg.Permission.Options.Rules = []permission.Rule{
		{Role: "user", Action: "booking.cancel", OwnedOnly: true, OwnedAttr: "owner"},
	}

	c := container.New(cfg)

	ctx := context.Background()
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
	idem, err := c.Idempotency()
	if err != nil {
		t.Fatalf("Idempotency: %v", err)
	}
	locker, err := c.Lock()
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	pay, err := c.Payment()
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	bill, err := c.Billing()
	if err != nil {
		t.Fatalf("Billing: %v", err)
	}
	q, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	crypt, err := c.Crypto()
	if err != nil {
		t.Fatalf("Crypto: %v", err)
	}
	ten, err := c.Tenant()
	if err != nil {
		t.Fatalf("Tenant: %v", err)
	}
	i18nInst, err := c.I18n()
	if err != nil {
		t.Fatalf("I18n: %v", err)
	}
	flags, err := c.Flag()
	if err != nil {
		t.Fatalf("Flag: %v", err)
	}
	perm, err := c.Permission()
	if err != nil {
		t.Fatalf("Permission: %v", err)
	}
	geoInst, err := c.Geo()
	if err != nil {
		t.Fatalf("Geo: %v", err)
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
	if err := api.EnsureExtraTables(ctx, database); err != nil {
		t.Fatalf("extra tables: %v", err)
	}

	t.Cleanup(func() {
		_ = c.Close(context.Background())
	})

	api.New(database, authInst, hasher, limiter, idem, locker, pay, bill, q, crypt, ten, i18nInst, flags, perm, geoInst).Routes(r)
	return testSetup{handler: r, queue: q}
}

// do sends a JSON request with optional bearer token and headers.
func do(t *testing.T, h http.Handler, method, path, token string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func register(t *testing.T, h http.Handler, email, pass, phone string) {
	t.Helper()
	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": email, "password": pass, "phone": phone,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: got %d body %s", email, rec.Code, rec.Body.String())
	}
}

func login(t *testing.T, h http.Handler, email, pass string) string {
	t.Helper()
	rec := do(t, h, "POST", "/api/login", "", map[string]string{
		"email": email, "password": pass,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: got %d body %s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login token missing: %v body %s", err, rec.Body.String())
	}
	return out.Token
}

// TestFullFlow covers register, space, book, double-book, cancel, refund, review.
func TestFullFlow(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler

	register(t, h, "host@example.com", "password123", "")
	// Duplicate register conflicts.
	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": "host@example.com", "password": "password123",
	}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: got %d body %s", rec.Code, rec.Body.String())
	}
	register(t, h, "guest@example.com", "password456", "+1-555-0100")
	hostToken := login(t, h, "host@example.com", "password123")
	guestToken := login(t, h, "guest@example.com", "password456")

	// Guest phone round-trips decrypted via /api/me.
	rec = do(t, h, "GET", "/api/me", guestToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: got %d body %s", rec.Code, rec.Body.String())
	}
	var me struct {
		Phone  string `json:"phone"`
		Tenant string `json:"tenant"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("me decode: %v", err)
	}
	if me.Phone != "+1-555-0100" {
		t.Fatalf("me phone = %q, want decrypted guest phone", me.Phone)
	}
	if me.Tenant == "" {
		t.Fatalf("me tenant empty, want resolved tenant")
	}

	// Host creates a Chicago space (matches geo fixture).
	rec = do(t, h, "POST", "/api/spaces", hostToken, map[string]any{
		"title": "Loft", "description": "sunny loft",
		"lat": 41.8781, "lng": -87.6298, "price_cents": 15000,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create space: got %d body %s", rec.Code, rec.Body.String())
	}
	var space struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &space); err != nil || space.ID == "" {
		t.Fatalf("space id missing: %v body %s", err, rec.Body.String())
	}

	// List shows the space; nearby q=Chicago finds it.
	rec = do(t, h, "GET", "/api/spaces", guestToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list spaces: got %d body %s", rec.Code, rec.Body.String())
	}
	var spaces []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spaces); err != nil || len(spaces) != 1 {
		t.Fatalf("list want 1, got %s err %v", rec.Body.String(), err)
	}
	rec = do(t, h, "GET", "/api/nearby?q=Chicago&radius_km=50", guestToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("nearby: got %d body %s", rec.Code, rec.Body.String())
	}
	var nearby []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &nearby); err != nil || len(nearby) != 1 {
		t.Fatalf("nearby want 1, got %s err %v", rec.Body.String(), err)
	}

	// Book with idempotency key + phone + tenant header.
	bookBody := map[string]string{
		"space_id": space.ID, "start_date": "2026-10-01", "end_date": "2026-10-05",
		"guest_phone": "+1-555-0100", "idempotency_key": "guest-trip-1",
	}
	rec = do(t, h, "POST", "/api/bookings", guestToken, bookBody, map[string]string{
		"Idempotency-Key": "guest-trip-1", "X-Tenant": "acme",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("book: got %d body %s", rec.Code, rec.Body.String())
	}
	var booked struct {
		Booking struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"booking"`
		Message         string `json:"message"`
		CheckoutVersion string `json:"checkout_version"`
		Tenant          string `json:"tenant"`
		PaymentID       string `json:"payment_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &booked); err != nil || booked.Booking.ID == "" {
		t.Fatalf("book decode: %v body %s", err, rec.Body.String())
	}
	if booked.Booking.Status != "confirmed" {
		t.Fatalf("book status = %q, want confirmed", booked.Booking.Status)
	}
	if !strings.Contains(booked.Message, "Booking confirmed") {
		t.Fatalf("book message = %q, want i18n confirmation", booked.Message)
	}
	if booked.CheckoutVersion != "v1" {
		t.Fatalf("checkout = %q, want v1 (flag off)", booked.CheckoutVersion)
	}
	if booked.PaymentID == "" {
		t.Fatalf("payment id empty, want stub charge link")
	}

	// Confirmation was dispatched.
	if _, err := ts.queue.Pop(context.Background(), "bookings.confirm"); err != nil {
		t.Fatalf("confirm dispatch missing: %v", err)
	}

	// Idempotent replay returns the same booking.
	rec = do(t, h, "POST", "/api/bookings", guestToken, bookBody, map[string]string{
		"Idempotency-Key": "guest-trip-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("replay: got %d body %s", rec.Code, rec.Body.String())
	}
	var replay struct {
		Booking struct {
			ID string `json:"id"`
		} `json:"booking"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil || replay.Booking.ID != booked.Booking.ID {
		t.Fatalf("replay id mismatch: %s", rec.Body.String())
	}

	// Double-book overlapping dates with a fresh key is rejected.
	rec = do(t, h, "POST", "/api/bookings", guestToken, map[string]string{
		"space_id": space.ID, "start_date": "2026-10-03", "end_date": "2026-10-07",
		"idempotency_key": "guest-trip-2",
	}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("double-book: got %d body %s", rec.Code, rec.Body.String())
	}

	// Cancel refunds and flips status.
	rec = do(t, h, "DELETE", "/api/bookings/"+booked.Booking.ID, guestToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: got %d body %s", rec.Code, rec.Body.String())
	}
	var cancelled struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelled); err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel status: %s err %v", rec.Body.String(), err)
	}
	if _, err := ts.queue.Pop(context.Background(), "bookings.cancel"); err != nil {
		t.Fatalf("cancel dispatch missing: %v", err)
	}

	// Review flow.
	rec = do(t, h, "POST", "/api/reviews", guestToken, map[string]any{
		"space_id": space.ID, "rating": 5, "body": "great stay",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review: got %d body %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/reviews?space_id="+space.ID, guestToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list reviews: got %d body %s", rec.Code, rec.Body.String())
	}
	var reviews []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &reviews); err != nil || len(reviews) != 1 {
		t.Fatalf("reviews want 1, got %s err %v", rec.Body.String(), err)
	}
}

// TestOwnerScoping ensures hosts own spaces and guests own bookings.
func TestOwnerScoping(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler
	_ = ts

	register(t, h, "alice@example.com", "password123", "")
	register(t, h, "bob@example.com", "password456", "")
	alice := login(t, h, "alice@example.com", "password123")
	bob := login(t, h, "bob@example.com", "password456")

	rec := do(t, h, "POST", "/api/spaces", alice, map[string]any{
		"title": "Cabin", "description": "woods", "lat": 41.8781, "lng": -87.6298, "price_cents": 9000,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create: got %d body %s", rec.Code, rec.Body.String())
	}
	var space struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &space); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Bob cannot patch or delete Alice's space; both 404.
	rec = do(t, h, "PATCH", "/api/spaces/"+space.ID, bob, map[string]string{"title": "hijack"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob patch foreign: got %d body %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "DELETE", "/api/spaces/"+space.ID, bob, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob delete foreign: got %d body %s", rec.Code, rec.Body.String())
	}

	// Bob books Alice's space; Alice cannot cancel Bob's booking (403).
	rec = do(t, h, "POST", "/api/bookings", bob, map[string]string{
		"space_id": space.ID, "start_date": "2026-11-01", "end_date": "2026-11-03",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bob book: got %d body %s", rec.Code, rec.Body.String())
	}
	var booked struct {
		Booking struct {
			ID string `json:"id"`
		} `json:"booking"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &booked); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rec = do(t, h, "DELETE", "/api/bookings/"+booked.Booking.ID, alice, nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("alice cancel foreign: got %d body %s", rec.Code, rec.Body.String())
	}

	// Booking lists are scoped: Alice sees none, Bob sees his.
	rec = do(t, h, "GET", "/api/bookings", alice, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("alice list: got %d", rec.Code)
	}
	var aliceBookings []any
	if err := json.Unmarshal(rec.Body.Bytes(), &aliceBookings); err != nil || len(aliceBookings) != 0 {
		t.Fatalf("alice list want empty, got %s", rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/bookings", bob, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bob list: got %d", rec.Code)
	}
	var bobBookings []any
	if err := json.Unmarshal(rec.Body.Bytes(), &bobBookings); err != nil || len(bobBookings) != 1 {
		t.Fatalf("bob list want 1, got %s", rec.Body.String())
	}
}

// TestTamperedToken ensures a modified JWT is rejected.
func TestTamperedToken(t *testing.T) {
	ts := newTestSetup(t)
	h := ts.handler
	_ = ts

	register(t, h, "alice@example.com", "password123", "")
	token := login(t, h, "alice@example.com", "password123")

	parts := strings.Split(token, ".")
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
	rec := do(t, h, "GET", "/api/spaces", tampered, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered: got %d body %s", rec.Code, rec.Body.String())
	}
}
