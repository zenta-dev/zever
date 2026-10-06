package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/examples/bookings/internal/api"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
)

// errCover is the generic backend failure cover stubs report.
var errCover = errors.New("cover: backend failure")

// stubFailDB fails every query and exec. Handlers must answer 4xx/5xx,
// never panic, on a broken database.
type stubFailDB struct{}

func (stubFailDB) Query(context.Context, string, ...any) (db.Rows, error) {
	return nil, errCover
}
func (stubFailDB) Exec(context.Context, string, ...any) (int64, error) {
	return 0, errCover
}
func (stubFailDB) Ping(context.Context) error  { return nil }
func (stubFailDB) Close(context.Context) error { return nil }
func (stubFailDB) Dialect() string             { return "sqlite" }

// coverDB delegates to a real database but fails queries/execs whose SQL
// contains the configured substring, pinning mid-flow error branches.
type coverDB struct {
	db.DB
	failQuery string
	failExec  string
}

func (c coverDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	if c.failQuery != "" && strings.Contains(strings.ToLower(query), strings.ToLower(c.failQuery)) {
		return nil, errCover
	}
	return c.DB.Query(ctx, query, args...)
}

func (c coverDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if c.failExec != "" && strings.Contains(strings.ToLower(query), c.failExec) {
		return 0, errCover
	}
	return c.DB.Exec(ctx, query, args...)
}

// stubAuth verifies as a scripted subject and fails Issue on demand.
type stubAuth struct {
	subject  string
	issueErr error
}

func (s stubAuth) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{Subject: s.subject}, nil
}
func (s stubAuth) Issue(context.Context, string, map[string]any, time.Duration) (auth.Token, error) {
	if s.issueErr != nil {
		return auth.Token{}, s.issueErr
	}
	return auth.Token{Value: "tok"}, nil
}
func (stubAuth) Revoke(context.Context, string) error { return nil }
func (stubAuth) Close() error                         { return nil }

// stubPassword verifies everything and fails Hash on demand.
type stubPassword struct{ hashErr error }

func (s stubPassword) Hash(context.Context, string) (string, error) {
	if s.hashErr != nil {
		return "", s.hashErr
	}
	return "hash", nil
}
func (stubPassword) Verify(context.Context, string, string) (bool, error) { return true, nil }
func (stubPassword) NeedsRehash(context.Context, string) (bool, error)    { return false, nil }

// stubLimit answers rate-limit checks with a scripted decision or error.
type stubLimit struct {
	allowed bool
	err     error
}

func (s stubLimit) Allow(context.Context, string, float64) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: s.allowed, RetryAfter: time.Second}, s.err
}
func (stubLimit) Reset(context.Context, string) error { return nil }
func (stubLimit) Close() error                        { return nil }
func (stubLimit) Name() string                        { return "stub" }

// stubIdem answers idempotency Begin with a scripted outcome.
type stubIdem struct{ beginErr error }

func (s stubIdem) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	if s.beginErr != nil {
		return idempotency.Outcome{}, s.beginErr
	}
	return idempotency.Outcome{}, nil
}
func (stubIdem) Complete(context.Context, string, []byte, []byte) error { return nil }
func (stubIdem) Forget(context.Context, string) error                   { return nil }
func (stubIdem) Close() error                                           { return nil }

// stubSingleLock is a no-op lease.
type stubSingleLock struct{}

func (stubSingleLock) Key() string                                 { return "stub" }
func (stubSingleLock) Extend(context.Context, time.Duration) error { return nil }
func (stubSingleLock) Unlock(context.Context) error                { return nil }

// stubLocker answers TryAcquire with a scripted held/error outcome.
type stubLocker struct {
	held bool
	err  error
}

func (s stubLocker) TryAcquire(context.Context, string, time.Duration) (lock.Lock, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if s.held {
		return nil, false, nil
	}
	return stubSingleLock{}, true, nil
}
func (stubLocker) Acquire(context.Context, string, time.Duration) (lock.Lock, error) {
	return nil, errCover
}
func (stubLocker) Close(context.Context) error { return nil }

// stubPayment fails CreatePayment on demand.
type stubPayment struct{ createErr error }

func (s stubPayment) CreatePayment(context.Context, payment.Request) (payment.Result, error) {
	if s.createErr != nil {
		return payment.Result{}, s.createErr
	}
	return payment.Result{ID: "pay_stub", Status: payment.PaymentSucceeded, Amount: 100, Currency: "USD"}, nil
}
func (stubPayment) Refund(context.Context, string, int64, string) error { return nil }
func (stubPayment) GetPayment(context.Context, string) (payment.Result, error) {
	return payment.Result{}, nil
}
func (stubPayment) WebhookEvent(context.Context, []byte, string) (payment.Event, error) {
	return payment.Event{}, nil
}
func (stubPayment) Close() error { return nil }

// stubCrypto fails Encrypt on demand and decrypts everything else.
type stubCrypto struct{ encErr error }

func (s stubCrypto) Encrypt(context.Context, []byte) ([]byte, error) {
	if s.encErr != nil {
		return nil, s.encErr
	}
	return []byte("ciphertext"), nil
}
func (stubCrypto) Decrypt(context.Context, []byte) ([]byte, error) {
	return []byte("plain"), nil
}
func (stubCrypto) Sign(context.Context, []byte) ([]byte, error) { return []byte("sig"), nil }
func (stubCrypto) Verify(context.Context, []byte, []byte) (bool, error) {
	return true, nil
}
func (stubCrypto) Mac(context.Context, []byte) ([]byte, error) { return []byte("mac"), nil }
func (stubCrypto) VerifyMac(context.Context, []byte, []byte) (bool, error) {
	return true, nil
}

// stubQueue fails Push on demand.
type stubQueue struct{ pushErr error }

func (s stubQueue) Push(context.Context, string, queue.Payload, queue.Headers) error {
	return s.pushErr
}
func (stubQueue) PushDelayed(context.Context, string, queue.Payload, queue.Headers, time.Duration) error {
	return nil
}
func (stubQueue) Pop(context.Context, string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}
func (stubQueue) Ack(context.Context, queue.Message) error        { return nil }
func (stubQueue) Nack(context.Context, queue.Message, bool) error { return nil }
func (stubQueue) Length(context.Context, string) (int64, error)   { return 0, nil }
func (stubQueue) IsEmpty(context.Context, string) (bool, error)   { return true, nil }
func (stubQueue) Close() error                                    { return nil }
func (stubQueue) Name() string                                    { return "stub" }

// stubFlag answers Bool with a scripted value or error.
type stubFlag struct {
	on  bool
	err error
}

func (s stubFlag) Bool(context.Context, string, bool) (bool, error) { return s.on, s.err }
func (stubFlag) String(context.Context, string, string) (string, error) {
	return "", nil
}
func (stubFlag) Int(context.Context, string, int) (int, error) { return 0, nil }
func (stubFlag) JSON(context.Context, string, any, any) error  { return nil }
func (stubFlag) Close() error                                  { return nil }

// stubI18n fails every translation.
type stubI18n struct{}

func (stubI18n) Translate(context.Context, string, string, map[string]string) (string, error) {
	return "", errCover
}
func (stubI18n) Locales(context.Context) ([]string, error) { return []string{"en"}, nil }
func (stubI18n) Close() error                              { return nil }

// stubTenantRepo resolves a scripted id or fails.
type stubTenantRepo struct {
	id  string
	err error
}

func (s stubTenantRepo) Resolve(context.Context, map[string]string) (string, error) {
	return s.id, s.err
}
func (s stubTenantRepo) Scoped(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}
func (stubTenantRepo) Close() error { return nil }

// stubPerm answers permission checks with a scripted decision or error.
type stubPerm struct {
	allowed bool
	err     error
}

func (s stubPerm) Can(context.Context, permission.Subject, string, permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: s.allowed}, s.err
}

// coverArgs mirrors api.New parameters so tests can swap one dependency
// for a stub while keeping the rest real.
type coverArgs struct {
	database db.DB
	authInst auth.Auth
	hasher   password.Hasher
	limiter  ratelimit.Limiter
	idem     idempotency.Store
	locker   lock.Locker
	pay      payment.Payment
	bill     billing.Billing
	q        queue.Queue
	crypt    crypto.Crypto
	ten      tenant.Tenant
	i18nInst i18n.I18n
	flags    flag.Flag
	perm     permission.Checker
	geoInst  geo.Geo
}

// coverSetup bundles a live container-backed API with every resolved dep,
// so cover tests can rebuild the API with stub swaps on the same database.
type coverSetup struct {
	handler http.Handler
	args    coverArgs
	db      db.DB
}

// newCoverSetup builds a fresh sqlite-backed API per test and exposes the
// resolved deps for stub-swap rebuilds.
func newCoverSetup(t *testing.T) coverSetup {
	t.Helper()

	authjwt.Register()
	billingstub.Register()
	cryptolocal.Register()
	dbsqlite.Register()
	flagstatic.Register()
	geostatic.Register()
	i18nembed.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	passwordargon2.Register()
	paymentstub.Register()
	permissionrbac.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	tenantsingle.Register()

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
	t.Cleanup(func() { _ = c.Close(t.Context()) })

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
	args := coverArgs{
		database: database,
		authInst: authInst,
		hasher:   hasher,
		limiter:  limiter,
		idem:     idem,
		locker:   locker,
		pay:      pay,
		bill:     bill,
		q:        q,
		crypt:    crypt,
		ten:      ten,
		i18nInst: i18nInst,
		flags:    flags,
		perm:     perm,
		geoInst:  geoInst,
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

	args.build(t, r)
	return coverSetup{handler: r, args: args, db: database}
}

// build routes the API described by args onto r.
func (a coverArgs) build(t *testing.T, r router.Router) {
	t.Helper()
	api.New(a.database, a.authInst, a.hasher, a.limiter, a.idem, a.locker,
		a.pay, a.bill, a.q, a.crypt, a.ten, a.i18nInst, a.flags, a.perm, a.geoInst).Routes(r)
}

// rebuild routes a copy of the setup args with mutate applied onto a fresh
// router sharing the same database.
func (s coverSetup) rebuild(t *testing.T, mutate func(*coverArgs)) http.Handler {
	t.Helper()
	args := s.args
	mutate(&args)
	r, err := router.Open(router.AdapterStdHTTP, router.Options{})
	if err != nil {
		t.Fatalf("router: %v", err)
	}
	args.build(t, r)
	return r
}

// coverRegister registers a user through h and fails the test on error.
func coverRegister(t *testing.T, h http.Handler, email, pass string) {
	t.Helper()
	rec := do(t, h, "POST", "/api/register", "", map[string]string{
		"email": email, "password": pass,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: got %d body %s", email, rec.Code, rec.Body.String())
	}
}

// coverLogin logs in and returns the token.
func coverLogin(t *testing.T, h http.Handler, email, pass string) string {
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

// coverSpace creates a space and returns its id.
func coverSpace(t *testing.T, h http.Handler, token string) string {
	t.Helper()
	rec := do(t, h, "POST", "/api/spaces", token, map[string]any{
		"title": "Loft", "description": "sunny", "lat": 41.8781, "lng": -87.6298, "price_cents": 15000,
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
	return space.ID
}

// coverBook creates a booking and returns its id.
func coverBook(t *testing.T, h http.Handler, token, spaceID, start, end string) string {
	t.Helper()
	rec := do(t, h, "POST", "/api/bookings", token, map[string]string{
		"space_id": spaceID, "start_date": start, "end_date": end,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("book: got %d body %s", rec.Code, rec.Body.String())
	}
	var booked struct {
		Booking struct {
			ID string `json:"id"`
		} `json:"booking"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &booked); err != nil || booked.Booking.ID == "" {
		t.Fatalf("book decode: %v body %s", err, rec.Body.String())
	}
	return booked.Booking.ID
}

// TestSpaceUpdateCoversPatch pins the space PATCH happy path plus every
// PATCH validation error.
func TestSpaceUpdateCoversPatch(t *testing.T) {
	s := newCoverSetup(t)
	h := s.handler

	coverRegister(t, h, "host@example.com", "password123")
	hostToken := coverLogin(t, h, "host@example.com", "password123")
	spaceID := coverSpace(t, h, hostToken)

	rec := do(t, h, "GET", "/api/spaces/"+spaceID, hostToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get space: got %d body %s", rec.Code, rec.Body.String())
	}

	rec = do(t, h, "PATCH", "/api/spaces/"+spaceID, hostToken, map[string]any{
		"title": "Renamed loft", "price_cents": 18000, "description": "brighter",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d body %s", rec.Code, rec.Body.String())
	}
	var patched struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		PriceCents  int64  `json:"price_cents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("patch decode: %v", err)
	}
	if patched.Title != "Renamed loft" || patched.PriceCents != 18000 || patched.Description != "brighter" {
		t.Fatalf("patched = %+v, want renamed fields", patched)
	}

	for _, tc := range []struct {
		name string
		body any
		want string
	}{
		{"empty title", map[string]string{"title": "  "}, "title must be"},
		{"title too long", map[string]string{"title": strings.Repeat("x", 201)}, "title must be"},
		{"negative price", map[string]any{"price_cents": -5}, "price_cents"},
		{"bad coords", map[string]any{"lat": 999.0, "lng": 999.0}, "invalid coordinates"},
	} {
		got := do(t, h, "PATCH", "/api/spaces/"+spaceID, hostToken, tc.body, nil)
		if got.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400 body %s", tc.name, got.Code, got.Body.String())
		}
		if !strings.Contains(got.Body.String(), tc.want) {
			t.Errorf("%s body = %s, want %q", tc.name, got.Body.String(), tc.want)
		}
	}

	rec = do(t, h, "PATCH", "/api/spaces/does-not-exist", hostToken, map[string]string{"title": "x"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("patch unknown = %d, want 404", rec.Code)
	}

	rec = do(t, h, "DELETE", "/api/spaces/"+spaceID, hostToken, nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 body %s", rec.Code, rec.Body.String())
	}
	if got := do(t, h, "GET", "/api/spaces/"+spaceID, hostToken, nil, nil); got.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", got.Code)
	}
	if got := do(t, h, "DELETE", "/api/spaces/"+spaceID, hostToken, nil, nil); got.Code != http.StatusNotFound {
		t.Errorf("delete twice = %d, want 404", got.Code)
	}
	rec = do(t, h, "GET", "/api/spaces", hostToken, nil, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("list after delete = %d %s, want empty", rec.Code, rec.Body.String())
	}
}

// TestNearbyVariants pins lat/lng search, radius filtering, and coordinate
// validation plus the geo-unavailable contract.
func TestNearbyVariants(t *testing.T) {
	s := newCoverSetup(t)
	h := s.handler

	coverRegister(t, h, "host@example.com", "password123")
	hostToken := coverLogin(t, h, "host@example.com", "password123")
	coverSpace(t, h, hostToken)

	rec := do(t, h, "GET", "/api/nearby?lat=41.8781&lng=-87.6298&radius_km=50", hostToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("nearby lat/lng: got %d body %s", rec.Code, rec.Body.String())
	}
	var nearby []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &nearby); err != nil || len(nearby) != 1 {
		t.Fatalf("nearby want 1, got %s err %v", rec.Body.String(), err)
	}

	rec = do(t, h, "GET", "/api/nearby?lat=0&lng=0&radius_km=1", hostToken, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("nearby far: got %d body %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("nearby far = %s, want empty", rec.Body.String())
	}

	for _, tc := range []struct {
		name, path, want string
	}{
		{"bad lng", "/api/nearby?lat=41.8&lng=abc", "lng required"},
		{"invalid coords", "/api/nearby?lat=999&lng=999", "invalid coordinates"},
		{"bad radius", "/api/nearby?lat=41.8&lng=-87.6&radius_km=abc", "invalid radius_km"},
	} {
		got := do(t, h, "GET", tc.path, hostToken, nil, nil)
		if got.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", tc.name, got.Code)
		}
		if !strings.Contains(got.Body.String(), tc.want) {
			t.Errorf("%s body = %s, want %q", tc.name, got.Body.String(), tc.want)
		}
	}

	noGeo := s.rebuild(t, func(a *coverArgs) { a.geoInst = nil })
	rec = do(t, noGeo, "GET", "/api/nearby?q=Chicago", hostToken, nil, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "geo unavailable") {
		t.Errorf("nearby no geo = %d %s, want 500 geo unavailable", rec.Code, rec.Body.String())
	}
}

// TestBookingGuards pins the ratelimit, idempotency, lock, and charge error
// contracts of the booking flow.
func TestBookingGuards(t *testing.T) {
	s := newCoverSetup(t)

	coverRegister(t, s.handler, "host@example.com", "password123")
	coverRegister(t, s.handler, "guest@example.com", "password456")
	hostToken := coverLogin(t, s.handler, "host@example.com", "password123")
	guestToken := coverLogin(t, s.handler, "guest@example.com", "password456")
	spaceID := coverSpace(t, s.handler, hostToken)
	book := func(start, end string) map[string]string {
		return map[string]string{"space_id": spaceID, "start_date": start, "end_date": end}
	}
	valid := book("2026-10-01", "2026-10-05")

	limited := s.rebuild(t, func(a *coverArgs) { a.limiter = stubLimit{err: errCover} })
	if rec := do(t, limited, "POST", "/api/bookings", guestToken, valid, nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("ratelimit error = %d, want 500", rec.Code)
	}
	denied := s.rebuild(t, func(a *coverArgs) { a.limiter = stubLimit{allowed: false} })
	if rec := do(t, denied, "POST", "/api/bookings", guestToken, valid, nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("ratelimit deny = %d, want 429", rec.Code)
	}

	inFlight := s.rebuild(t, func(a *coverArgs) { a.idem = stubIdem{beginErr: idempotency.ErrInProgress} })
	if rec := do(t, inFlight, "POST", "/api/bookings", guestToken, valid, nil); rec.Code != http.StatusConflict {
		t.Errorf("idempotency in progress = %d, want 409", rec.Code)
	}
	mismatch := s.rebuild(t, func(a *coverArgs) { a.idem = stubIdem{beginErr: idempotency.ErrKeyMismatch} })
	if rec := do(t, mismatch, "POST", "/api/bookings", guestToken, valid, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("idempotency mismatch = %d, want 400", rec.Code)
	}
	idemErr := s.rebuild(t, func(a *coverArgs) { a.idem = stubIdem{beginErr: errCover} })
	if rec := do(t, idemErr, "POST", "/api/bookings", guestToken, valid, nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("idempotency error = %d, want 500", rec.Code)
	}

	lockErr := s.rebuild(t, func(a *coverArgs) { a.locker = stubLocker{err: errCover} })
	if rec := do(t, lockErr, "POST", "/api/bookings", guestToken, book("2026-10-06", "2026-10-08"), nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("lock error = %d, want 500", rec.Code)
	}
	held := s.rebuild(t, func(a *coverArgs) { a.locker = stubLocker{held: true} })
	if rec := do(t, held, "POST", "/api/bookings", guestToken, book("2026-10-09", "2026-10-11"), nil); rec.Code != http.StatusConflict {
		t.Errorf("lock held = %d, want 409", rec.Code)
	}

	noCharge := s.rebuild(t, func(a *coverArgs) { a.pay = stubPayment{createErr: errCover} })
	if rec := do(t, noCharge, "POST", "/api/bookings", guestToken, book("2026-10-12", "2026-10-14"), nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("charge error = %d, want 500", rec.Code)
	}

	noPhone := s.rebuild(t, func(a *coverArgs) { a.crypt = stubCrypto{encErr: errCover} })
	phoneBody := map[string]string{"space_id": spaceID, "start_date": "2026-11-01", "end_date": "2026-11-03", "guest_phone": "+1-555-0100"}
	if rec := do(t, noPhone, "POST", "/api/bookings", guestToken, phoneBody, nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("phone encrypt error = %d, want 500 body %s", rec.Code, rec.Body.String())
	}

	noQueue := s.rebuild(t, func(a *coverArgs) { a.q = stubQueue{pushErr: errCover} })
	queueBody := map[string]string{"space_id": spaceID, "start_date": "2026-12-01", "end_date": "2026-12-03"}
	if rec := do(t, noQueue, "POST", "/api/bookings", guestToken, queueBody, nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("queue error = %d, want 500 body %s", rec.Code, rec.Body.String())
	}
}

// TestBookingKeys pins the header, body, and default idempotency key shapes
// plus locale selection and checkout/tenant/i18n fallbacks.
func TestBookingKeys(t *testing.T) {
	s := newCoverSetup(t)
	h := s.handler

	coverRegister(t, h, "host@example.com", "password123")
	coverRegister(t, h, "guest@example.com", "password456")
	hostToken := coverLogin(t, h, "host@example.com", "password123")
	guestToken := coverLogin(t, h, "guest@example.com", "password456")
	spaceID := coverSpace(t, h, hostToken)

	bodyKey := map[string]string{
		"space_id": spaceID, "start_date": "2026-10-01", "end_date": "2026-10-05",
		"idempotency_key": "body-only-key",
	}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, bodyKey, nil); rec.Code != http.StatusCreated {
		t.Fatalf("body key first: got %d body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, bodyKey, nil); rec.Code != http.StatusOK {
		t.Fatalf("body key replay: got %d body %s", rec.Code, rec.Body.String())
	}

	defaultKey := map[string]string{"space_id": spaceID, "start_date": "2026-11-01", "end_date": "2026-11-03"}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, defaultKey, nil); rec.Code != http.StatusCreated {
		t.Fatalf("default key first: got %d body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, defaultKey, nil); rec.Code != http.StatusOK {
		t.Fatalf("default key replay: got %d body %s", rec.Code, rec.Body.String())
	}

	localeBody := map[string]string{"space_id": spaceID, "start_date": "2026-12-01", "end_date": "2026-12-03"}
	if rec := do(t, h, "POST", "/api/bookings?locale=fr", guestToken, localeBody, nil); rec.Code != http.StatusCreated {
		t.Errorf("locale query booking = %d %s, want 201", rec.Code, rec.Body.String())
	}
	langBody := map[string]string{"space_id": spaceID, "start_date": "2026-12-05", "end_date": "2026-12-07"}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, langBody,
		map[string]string{"Accept-Language": "fr-FR,fr;q=0.9"}); rec.Code != http.StatusCreated {
		t.Errorf("accept-language booking = %d %s, want 201", rec.Code, rec.Body.String())
	}
	blankBody := map[string]string{"space_id": spaceID, "start_date": "2026-12-09", "end_date": "2026-12-11"}
	if rec := do(t, h, "POST", "/api/bookings", guestToken, blankBody,
		map[string]string{"Accept-Language": " ;"}); rec.Code != http.StatusCreated {
		t.Errorf("blank language booking = %d %s, want 201", rec.Code, rec.Body.String())
	}

	noFlag := s.rebuild(t, func(a *coverArgs) { a.flags = nil })
	rec := do(t, noFlag, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-01-01", "end_date": "2027-01-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"checkout_version":"v1"`) {
		t.Errorf("nil flag checkout = %d %s, want v1", rec.Code, rec.Body.String())
	}

	flagOn := s.rebuild(t, func(a *coverArgs) { a.flags = stubFlag{on: true} })
	rec = do(t, flagOn, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-02-01", "end_date": "2027-02-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"checkout_version":"v2"`) {
		t.Errorf("flag on checkout = %d %s, want v2", rec.Code, rec.Body.String())
	}

	flagErr := s.rebuild(t, func(a *coverArgs) { a.flags = stubFlag{err: errCover} })
	rec = do(t, flagErr, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-03-01", "end_date": "2027-03-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"checkout_version":"v1"`) {
		t.Errorf("flag error checkout = %d %s, want v1", rec.Code, rec.Body.String())
	}

	noI18n := s.rebuild(t, func(a *coverArgs) { a.i18nInst = nil })
	rec = do(t, noI18n, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-04-01", "end_date": "2027-04-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "Booking confirmed for") {
		t.Errorf("nil i18n message = %d %s, want fallback", rec.Code, rec.Body.String())
	}

	badI18n := s.rebuild(t, func(a *coverArgs) { a.i18nInst = stubI18n{} })
	rec = do(t, badI18n, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-05-01", "end_date": "2027-05-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "Booking confirmed for") {
		t.Errorf("error i18n message = %d %s, want fallback", rec.Code, rec.Body.String())
	}

	badTenant := s.rebuild(t, func(a *coverArgs) { a.ten = stubTenantRepo{err: errCover} })
	rec = do(t, badTenant, "POST", "/api/bookings", guestToken,
		map[string]string{"space_id": spaceID, "start_date": "2027-06-01", "end_date": "2027-06-03"}, nil)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"tenant":""`) {
		t.Errorf("tenant error = %d %s, want empty tenant", rec.Code, rec.Body.String())
	}
}

// TestBookingDBFailures pins the mid-flow database error branches: overlap
// check, booking insert, payment link, phone store, and space lookup.
func TestBookingDBFailures(t *testing.T) {
	s := newCoverSetup(t)

	coverRegister(t, s.handler, "host@example.com", "password123")
	coverRegister(t, s.handler, "guest@example.com", "password456")
	hostToken := coverLogin(t, s.handler, "host@example.com", "password123")
	guestToken := coverLogin(t, s.handler, "guest@example.com", "password456")
	spaceID := coverSpace(t, s.handler, hostToken)

	failSpace := s.rebuild(t, func(a *coverArgs) { a.database = stubFailDB{} })
	rec := do(t, failSpace, "POST", "/api/bookings", guestToken, map[string]string{
		"space_id": spaceID, "start_date": "2026-10-01", "end_date": "2026-10-05",
	}, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "space lookup failed") {
		t.Errorf("space lookup = %d %s, want 500", rec.Code, rec.Body.String())
	}
}

// TestDatabaseErrors pins 4xx/5xx for every route family when the database
// fails, using a stub auth subject so the DB fault is isolated.
func TestDatabaseErrors(t *testing.T) {
	s := newCoverSetup(t)

	coverRegister(t, s.handler, "edge@example.com", "password123")
	token := coverLogin(t, s.handler, "edge@example.com", "password123")
	spaceID := coverSpace(t, s.handler, token)
	bookingID := coverBook(t, s.handler, token, spaceID, "2026-10-01", "2026-10-05")

	h := s.rebuild(t, func(a *coverArgs) {
		a.database = stubFailDB{}
		a.authInst = stubAuth{subject: "edge"}
	})

	tests := []struct {
		method, path string
		body         any
		token        string
		want         int
		msg          string
	}{
		{"POST", "/api/register", map[string]string{"email": "a@b.c", "password": "password123"}, "", 500, "lookup failed"},
		{"POST", "/api/login", map[string]string{"email": "a@b.c", "password": "password123"}, "", 401, "invalid credentials"},
		{"GET", "/api/me", nil, "tok", 500, "lookup failed"},
		{"POST", "/api/spaces", map[string]any{"title": "T", "lat": 1.0, "lng": 2.0}, "tok", 500, "create failed"},
		{"GET", "/api/spaces", nil, "tok", 500, "list failed"},
		{"GET", "/api/spaces/" + spaceID, nil, "tok", 500, "get failed"},
		{"PATCH", "/api/spaces/" + spaceID, map[string]string{"title": "x"}, "tok", 500, "get failed"},
		{"DELETE", "/api/spaces/" + spaceID, nil, "tok", 500, "delete failed"},
		{"GET", "/api/nearby?lat=41.8&lng=-87.6", nil, "tok", 500, "list failed"},
		{"POST", "/api/bookings", map[string]string{"space_id": "x", "start_date": "2026-10-01", "end_date": "2026-10-05"}, "tok", 500, "space lookup failed"},
		{"GET", "/api/bookings", nil, "tok", 500, "list failed"},
		{"DELETE", "/api/bookings/" + bookingID, nil, "tok", 500, "lookup failed"},
		{"POST", "/api/reviews", map[string]any{"space_id": "x", "rating": 5, "body": "b"}, "tok", 500, "space lookup failed"},
		{"GET", "/api/reviews", nil, "tok", 500, "list failed"},
	}
	for _, tc := range tests {
		rec := do(t, h, tc.method, tc.path, tc.token, tc.body, nil)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d body %s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.msg) {
			t.Errorf("%s %s body = %s, want %q", tc.method, tc.path, rec.Body.String(), tc.msg)
		}
	}
}

// TestUserEdge pins credential, hashing, issue, phone, and profile error
// contracts.
func TestUserEdge(t *testing.T) {
	s := newCoverSetup(t)
	h := s.handler

	coverRegister(t, h, "u@example.com", "password123")
	rec := do(t, h, "POST", "/api/login", "", map[string]string{"email": "u@example.com", "password": "wrongpass1"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password = %d, want 401", rec.Code)
	}
	rec = do(t, h, "POST", "/api/login", "", map[string]string{"email": "nobody@example.com", "password": "password123"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown email = %d, want 401", rec.Code)
	}
	token := coverLogin(t, h, "u@example.com", "password123")

	issueErr := s.rebuild(t, func(a *coverArgs) { a.authInst = stubAuth{subject: "u", issueErr: errCover} })
	rec = do(t, issueErr, "POST", "/api/login", "", map[string]string{"email": "u@example.com", "password": "password123"}, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "issue failed") {
		t.Errorf("issue error = %d %s, want 500", rec.Code, rec.Body.String())
	}

	hashErr := s.rebuild(t, func(a *coverArgs) {
		a.authInst = stubAuth{subject: "u"}
		a.hasher = stubPassword{hashErr: errCover}
	})
	rec = do(t, hashErr, "POST", "/api/register", "", map[string]string{"email": "n@example.com", "password": "password123"}, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "hash failed") {
		t.Errorf("hash error = %d %s, want 500", rec.Code, rec.Body.String())
	}

	phoneErr := s.rebuild(t, func(a *coverArgs) { a.crypt = stubCrypto{encErr: errCover} })
	rec = do(t, phoneErr, "POST", "/api/register", "", map[string]string{
		"email": "p@example.com", "password": "password123", "phone": "+1-555-0100",
	}, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "phone encrypt failed") {
		t.Errorf("phone encrypt = %d %s, want 500", rec.Code, rec.Body.String())
	}

	noCrypto := s.rebuild(t, func(a *coverArgs) {
		a.crypt = nil
		a.ten = nil
	})
	rec = do(t, noCrypto, "GET", "/api/me", token, nil, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tenant":""`) {
		t.Errorf("me minimal = %d %s, want empty tenant", rec.Code, rec.Body.String())
	}

	if _, err := s.db.Exec(t.Context(), `DELETE FROM users`); err != nil {
		t.Fatalf("delete users: %v", err)
	}
	if rec := do(t, h, "GET", "/api/me", token, nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("me after delete = %d, want 404", rec.Code)
	}

	emptySub := s.rebuild(t, func(a *coverArgs) { a.authInst = stubAuth{} })
	if rec := do(t, emptySub, "GET", "/api/me", "tok", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("empty subject = %d, want 401", rec.Code)
	}
}

// TestReviewEdge pins rating boundaries, missing fields, filters, and the
// cancel permission-error contract.
func TestReviewEdge(t *testing.T) {
	s := newCoverSetup(t)
	h := s.handler

	coverRegister(t, h, "host@example.com", "password123")
	coverRegister(t, h, "guest@example.com", "password456")
	hostToken := coverLogin(t, h, "host@example.com", "password123")
	guestToken := coverLogin(t, h, "guest@example.com", "password456")
	spaceID := coverSpace(t, h, hostToken)

	for _, rating := range []int64{1, 5} {
		rec := do(t, h, "POST", "/api/reviews", guestToken, map[string]any{
			"space_id": spaceID, "rating": rating, "body": "ok",
		}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("rating %d = %d %s", rating, rec.Code, rec.Body.String())
		}
	}
	rec := do(t, h, "POST", "/api/reviews", guestToken, map[string]any{"rating": 5, "body": "x"}, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "space_id required") {
		t.Errorf("missing space = %d %s, want 400", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/reviews?space_id=no-such-space", guestToken, nil, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("filter mismatch = %d %s, want empty", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/api/reviews", guestToken, nil, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), spaceID) {
		t.Errorf("list all = %d %s, want reviews", rec.Code, rec.Body.String())
	}

	bookingID := coverBook(t, h, guestToken, spaceID, "2026-10-01", "2026-10-05")
	permErr := s.rebuild(t, func(a *coverArgs) { a.perm = stubPerm{err: errCover} })
	if rec := do(t, permErr, "DELETE", "/api/bookings/"+bookingID, guestToken, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("permission error cancel = %d, want 403", rec.Code)
	}
	noPerm := s.rebuild(t, func(a *coverArgs) { a.perm = nil })
	if rec := do(t, noPerm, "DELETE", "/api/bookings/"+bookingID, hostToken, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("nil permission foreign cancel = %d, want 403 body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, noPerm, "DELETE", "/api/bookings/"+bookingID, guestToken, nil, nil); rec.Code != http.StatusOK {
		t.Errorf("nil permission owner cancel = %d, want 200 body %s", rec.Code, rec.Body.String())
	}
}

// TestEnsureExtraTablesError pins the error branch of EnsureExtraTables.
func TestEnsureExtraTablesError(t *testing.T) {
	if err := api.EnsureExtraTables(t.Context(), stubFailDB{}); err == nil {
		t.Fatal("EnsureExtraTables failing db: want error")
	}
}

// TestBookingInsertFailures pins booking insert, payment link, and phone
// store failures with query-precise faults.
func TestBookingInsertFailures(t *testing.T) {
	s := newCoverSetup(t)

	coverRegister(t, s.handler, "host@example.com", "password123")
	coverRegister(t, s.handler, "guest@example.com", "password456")
	hostToken := coverLogin(t, s.handler, "host@example.com", "password123")
	guestToken := coverLogin(t, s.handler, "guest@example.com", "password456")
	spaceID := coverSpace(t, s.handler, hostToken)

	tests := []struct {
		name      string
		failExec  string
		failQuery string
		body      map[string]string
		want      string
	}{
		{"overlap check", "", "from bookings where", map[string]string{
			"space_id": spaceID, "start_date": "2026-10-01", "end_date": "2026-10-05"}, "availability check failed"},
		{"booking insert", "insert into bookings", "", map[string]string{
			"space_id": spaceID, "start_date": "2026-10-10", "end_date": "2026-10-12"}, "booking failed"},
		{"payment link", "insert into booking_payments", "", map[string]string{
			"space_id": spaceID, "start_date": "2026-11-01", "end_date": "2026-11-03"}, "booking failed"},
		{"phone store", "insert into booking_phones", "", map[string]string{
			"space_id": spaceID, "start_date": "2026-12-01", "end_date": "2026-12-03", "guest_phone": "+1-555-0100"}, "phone store failed"},
	}
	for _, tc := range tests {
		h := s.rebuild(t, func(a *coverArgs) {
			a.database = coverDB{DB: s.db, failExec: tc.failExec, failQuery: tc.failQuery}
		})
		rec := do(t, h, "POST", "/api/bookings", guestToken, tc.body, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s = %d, want 500 body %s", tc.name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s body = %s, want %q", tc.name, rec.Body.String(), tc.want)
		}
	}
}
