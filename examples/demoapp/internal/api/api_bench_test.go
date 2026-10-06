package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/examples/demoapp/internal/api"
	"github.com/zenta-dev/zever/examples/demoapp/locales"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	searchdb "github.com/zenta-dev/zever/adapters/search/db"
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"

	aianthropic "github.com/zenta-dev/zever/adapters/ai/anthropic"
	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
)

// benchSetup builds a fresh handler for benchmarks with per-bench temp dirs.
func benchSetup(b *testing.B) http.Handler {
	b.Helper()

	tmp := b.TempDir()
	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(tmp, "bench.db")
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
		b.Fatalf("flag path: %v", err)
	}
	cfg.Flag.Options.Static.Path = flagPath
	geoPath, err := filepath.Abs(filepath.Join("..", "..", "data", "cities.json"))
	if err != nil {
		b.Fatalf("geo path: %v", err)
	}
	cfg.Geo.Options.Path = geoPath
	cfg.Search.Options.DSN = filepath.Join(tmp, "search.db")
	cfg.VectorStore.Options.DSN = filepath.Join(tmp, "vectors.db")
	cfg.VectorStore.Options.Dimension = 8
	cfg.Storage.Options.Root = filepath.Join(tmp, "storage")
	cfg.Media.Options.Root = filepath.Join(tmp, "media")

	documentlocal.Register()
	medialocal.Register()
	authjwt.Register()
	billingstub.Register()
	dbsqlite.Register()
	aianthropic.Register()
	analyticslog.Register()
	cachememory.Register()
	cryptolocal.Register()
	eventbusmemory.Register()
	flagstatic.Register()
	geostatic.Register()
	i18nembed.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	logslog.Register()
	mailerlog.Register()
	notificationlog.Register()
	observabilitystdout.Register()
	paymentstub.Register()
	permissionrbac.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	schedulerembedded.Register()
	secretsenv.Register()
	sessionmemory.Register()
	storagelocal.Register()
	tenantsingle.Register()
	webhookhttp.Register()
	workflowmemory.Register()
	passwordargon2.Register()
	searchdb.Register()
	vectorstoredb.Register()

	c := container.New(cfg)
	ctx := b.Context()
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
	logger, err := c.Log()
	if err != nil {
		b.Fatalf("Log: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		b.Fatalf("Router: %v", err)
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
	api.RegisterDemoWorkflow(d.Workflow)

	raw, err := os.ReadFile(filepath.Join("testdata", "schema.sql"))
	if err != nil {
		b.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			b.Fatalf("migrate: %v", err)
		}
	}
	b.Cleanup(func() { _ = c.Close(b.Context()) })
	api.New(d).Routes(r)
	return r
}

// BenchmarkListProducts measures the public product listing hot path.
func BenchmarkListProducts(b *testing.B) {
	h := benchSetup(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/products", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("list: %d", rec.Code)
		}
	}
}

// BenchmarkHealth measures the health endpoint hot path.
func BenchmarkHealth(b *testing.B) {
	h := benchSetup(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("health: %d", rec.Code)
		}
	}
}
