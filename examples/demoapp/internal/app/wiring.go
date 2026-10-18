// Package app wires this project's service instances.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors; Modules mounts the project's serve
// modules. Add batteries with `zever add <battery>`: it only inserts Ref
// vars and Provider entries here and never rewrites your code. Tune an
// instance with container.WithOptions entries in Providers.
package app

import (
	"net/http"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/examples/demoapp/internal/api"
	"github.com/zenta-dev/zever/serve"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
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
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
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
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
)

// DB names the default sqlite instance (data/app.db unless DB_PATH overrides).
var DB = db.DefaultRef()

// Analytics names the default log analytics instance.
var Analytics = analytics.DefaultRef()

// Auth names the default jwt auth instance.
var Auth = auth.DefaultRef()

// Billing names the default stub billing instance.
var Billing = billing.DefaultRef()

// Cache names the default memory cache instance.
var Cache = cache.DefaultRef()

// Crypto names the default local crypto instance.
var Crypto = crypto.DefaultRef()

// Document names the default local document instance.
var Document = document.DefaultRef()

// EventBus names the default memory eventbus instance.
var EventBus = eventbus.DefaultRef()

// Flag names the default static flag instance.
var Flag = flag.DefaultRef()

// Geo names the default static geo instance.
var Geo = geo.DefaultRef()

// I18n names the default embed i18n instance.
var I18n = i18n.DefaultRef()

// Idempotency names the default memory idempotency instance.
var Idempotency = idempotency.DefaultRef()

// Lock names the default memory lock instance.
var Lock = lock.DefaultRef()

// Log names the default slog logger instance.
var Log = log.DefaultRef()

// Mailer names the default log mailer instance.
var Mailer = mailer.DefaultRef()

// Media names the default local media instance.
var Media = media.DefaultRef()

// Notification names the default log notification instance.
var Notification = notification.DefaultRef()

// Observability names the default stdout provider instance.
var Observability = observability.DefaultRef()

// Outbox names the default db outbox instance.
var Outbox = outbox.DefaultRef()

// Password names the default argon2 hasher instance.
var Password = password.DefaultRef()

// Payment names the default stub payment instance.
var Payment = payment.DefaultRef()

// Permission names the default rbac checker instance.
var Permission = permission.DefaultRef()

// Queue names the default memory queue instance.
var Queue = queue.DefaultRef()

// RateLimit names the default memory ratelimit instance.
var RateLimit = ratelimit.DefaultRef()

// Router names the default stdhttp router instance.
var Router = router.DefaultRef()

// Scheduler names the default embedded scheduler instance.
var Scheduler = scheduler.DefaultRef()

// Search names the default db search instance.
var Search = search.DefaultRef()

// Secrets names the default env secrets instance.
var Secrets = secrets.DefaultRef()

// Session names the default memory session instance.
var Session = session.DefaultRef()

// Storage names the default local storage instance.
var Storage = storage.DefaultRef()

// Tenant names the default single tenant instance.
var Tenant = tenant.DefaultRef()

// VectorStore names the default db vectorstore instance.
var VectorStore = vectorstore.DefaultRef()

// Webhook names the default http webhook instance.
var Webhook = webhook.DefaultRef()

// Workflow names the default memory workflow instance.
var Workflow = workflow.DefaultRef()

// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps, cycles)
// and resolves each instance lazily on first use. Runtime defaults apply
// only when the environment leaves a field unset, so env always wins.
func Providers() []container.Provider {
	return []container.Provider{
		container.Provide(DB, func(o db.Options) (db.DB, error) {
			if o.Path == "" {
				o.Path = DefaultDBPath
			}

			return dbsqlite.New(o)
		}),
		container.Provide(Analytics, analyticslog.New),
		container.Provide(Auth, authjwt.New),
		container.Provide(Billing, func(_ billing.Options) (billing.Billing, error) {
			return billingstub.New(), nil
		}),
		container.Provide(Cache, cachememory.New),
		container.Provide(Crypto, cryptolocal.New),
		container.Provide(Document, documentlocal.New),
		container.Provide(EventBus, eventbusmemory.New),
		container.Provide(Flag, flagstatic.New),
		container.Provide(Geo, geostatic.New),
		container.Provide(I18n, i18nembed.New),
		container.Provide(Idempotency, idempotencymemory.New),
		container.Provide(Lock, lockmemory.New),
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Mailer, mailerlog.New),
		container.Provide(Media, medialocal.New),
		container.Provide(Notification, notificationlog.New),
		container.Provide(Observability, observabilitystdout.New),
		container.Provide(Outbox, func(_ outbox.Options) (outbox.Store, error) {
			return nil, nil
		}),
		container.Provide(Password, passwordargon2.New),
		container.Provide(Payment, func(_ payment.Options) (payment.Payment, error) {
			return paymentstub.New(payment.Options{})
		}),
		container.Provide(Permission, permissionrbac.New),
		container.Provide(Queue, queuememory.New),
		container.Provide(RateLimit, ratelimitmemory.New),
		container.Provide(Router, routerstdhttp.New),
		container.Provide(Scheduler, schedulerembedded.New),
		container.Provide(Search, func(_ search.Options) (search.Search, error) {
			return nil, nil
		}),
		container.Provide(Secrets, func(o secrets.Options) (secrets.Secrets, error) {
			return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
		}),
		container.Provide(Session, sessionmemory.New),
		container.Provide(Storage, storagelocal.New),
		container.Provide(Tenant, tenantsingle.New),
		container.Provide(VectorStore, vectorstoredb.New),
		container.Provide(Webhook, webhookhttp.New),
		container.Provide(Workflow, workflowmemory.New),
	}
}

// Modules returns the serve modules mounted at boot, after the shared gRPC
// server (when enabled) is built. Each module registers its routes on the
// resolved router via Mount.
func Modules() func(*container.Container) []serve.Module {
	return func(*container.Container) []serve.Module {
		return []serve.Module{
			&demoModule{},
		}
	}
}

// demoModule mounts the demoapp HTTP API onto the resolved router,
// resolving every battery best-effort from the container. Batteries that
// fail to resolve stay nil and handlers answer 501 for them.
type demoModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *demoModule) Name() string {
	return "demo"
}

// Mount registers the demoapp API routes on r.
func (m *demoModule) Mount(r router.Router, c *container.Container) error {
	database, err := c.DB()
	if err != nil {
		return err
	}

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	hasher, err := c.Password()
	if err != nil {
		return err
	}

	logger, err := c.Log()
	if err != nil {
		return err
	}

	d := api.Deps{
		DB:       database,
		Auth:     authInst,
		Password: hasher,
		Log:      logger,
	}

	// Optional batteries: stay nil when resolution fails; handlers answer 501.
	if v, err := c.Cache(); err == nil {
		d.Cache = v
	}
	if v, err := c.Flag(); err == nil {
		d.Flag = v
	}
	if v, err := c.Permission(); err == nil {
		d.Permission = v
	}
	if v, err := c.RateLimit(); err == nil {
		d.RateLimit = v
	}
	if v, err := c.Lock(); err == nil {
		d.Lock = v
	}
	if v, err := c.Idempotency(); err == nil {
		d.Idempotency = v
	}
	if v, err := c.Session(); err == nil {
		d.Session = v
	}
	if v, err := c.Queue(); err == nil {
		d.Queue = v
	}
	if v, err := c.Job(); err == nil {
		d.Job = v
	}
	if v, err := c.EventBus(); err == nil {
		d.EventBus = v
	}
	if v, err := c.Search(); err == nil {
		d.Search = v
	}
	if v, err := c.VectorStore(); err == nil {
		d.VectorStore = v
	}
	if v, err := c.Storage(); err == nil {
		d.Storage = v
	}
	if v, err := c.Media(); err == nil {
		d.Media = v
	}
	if v, err := c.Geo(); err == nil {
		d.Geo = v
	}
	if v, err := c.I18n(); err == nil {
		d.I18n = v
	}
	if v, err := c.Crypto(); err == nil {
		d.Crypto = v
	}
	if v, err := c.Secrets(); err == nil {
		d.Secrets = v
	}
	if v, err := c.Notification(); err == nil {
		d.Notification = v
	}
	if v, err := c.Mailer(); err == nil {
		d.Mailer = v
	}
	if v, err := c.Webhook(); err == nil {
		d.Webhook = v
	}
	if v, err := c.Workflow(); err == nil {
		d.Workflow = v
	}
	if v, err := c.Observability(); err == nil {
		d.Observability = v
	}
	if v, err := c.Analytics(); err == nil {
		d.Analytics = v
	}
	if v, err := c.Payment(); err == nil {
		d.Payment = v
	}
	if v, err := c.Billing(); err == nil {
		d.Billing = v
	}
	if v, err := c.Document(); err == nil {
		d.Document = v
	}
	if v, err := c.Tenant(); err == nil {
		d.Tenant = v
	}

	api.RegisterDemoWorkflow(d.Workflow)

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	api.New(d).Routes(r)

	return nil
}
